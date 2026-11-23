package handler

import (
	"context"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/costofliving"
	"github.com/omareloui/tyche/internal/countries"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/web/views"
)

// The FIRE calculator's assumptions when left empty.
const (
	fireReturn    = 5.0
	fireSWR       = 4.0
	fireVol       = 15.0
	fireRetireAge = 65
)

// FireCalc is the FIRE calculator, its form from the query string, the
// amounts left empty taken from the user's money.
func (h Handler) FireCalc(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	b, err := h.repo.GetBalances(ctx)
	if err != nil {
		return nil, err
	}
	d, err := h.fireData(ctx, r.URL.Query())
	if err != nil {
		return nil, err
	}
	return views.FireCalc(getClaims(ctx), b, d), nil
}

// FireCalcResult is the calculator's result, worked out on every change of
// the form, keeping the form in the page's url.
func (h Handler) FireCalcResult(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	d, err := h.fireData(r.Context(), r.URL.Query())
	if err != nil {
		return nil, err
	}
	w.Header().Set("HX-Replace-Url", views.FireURL(d.Form))
	return views.FireResultSwap(d), nil
}

func parseFireForm(v url.Values) views.FireForm {
	get := func(k string) string { return strings.TrimSpace(v.Get(k)) }
	return views.FireForm{
		Portfolio: get("portfolio"),
		Spending:  get("spending"),
		Saving:    get("saving"),
		PartTime:  get("part_time"),
		Return:    get("return"),
		SWR:       get("swr"),
		Vol:       get("vol"),
		RetireAge: get("retire_age"),
		Excluded:  get("excluded") != "",
		Goals:     get("goals") != "",
	}
}

// parseFireAmount reads an amount: empty is not typed, 0 is.
func parseFireAmount(s string) (v float64, typed, ok bool) {
	if s == "" {
		return 0, false, true
	}
	if strings.Trim(s, "0.,") == "" {
		return 0, true, true
	}
	v, ok = parseColAmount(s)
	return v, true, ok
}

// parseFirePct reads a percent (signed), def when empty.
func parseFirePct(s string, def float64) (float64, bool) {
	if s == "" {
		return def / 100, true
	}
	v, err := strconv.ParseFloat(strings.TrimSuffix(strings.ReplaceAll(s, ",", "."), "%"), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v / 100, true
}

// fireData works the calculator out of the form: the amounts left empty
// from the user's money (the saving from the income less the spending,
// typed or not, else the settings' salary), the assumptions from their
// defaults, the ages from the profile's birth year.
func (h Handler) fireData(ctx context.Context, v url.Values) (*views.FireData, error) {
	claims := getClaims(ctx)
	today := h.userToday(ctx)
	d := &views.FireData{Form: parseFireForm(v), Currency: claims.DefaultCurrency}

	usr, err := h.repo.GetUserByOAuthID(ctx, claims.OAuthID)
	if err != nil {
		return nil, err
	}
	d.BirthYear = usr.BirthYear
	d.Age = models.AgeIn(usr.BirthYear, today)

	d.Money, err = h.repo.GetFireMoney(ctx, &claims.DefaultCurrency, today, models.AccountsSummaryWith{Excluded: d.Form.Excluded, Goals: d.Form.Goals})
	if err != nil {
		return nil, err
	}
	d.PortfolioDef = d.Money.Portfolio
	d.SpendingDef = d.Money.AvgSpending()

	if c, ok := countries.FromTimezone(claims.Timezone); ok {
		data := costofliving.Current()
		if _, last := data.InflationYears(c.ISO3); last > 0 {
			if pct, ok := data.Inflation(c.ISO3)[min(last, today.Year())]; ok {
				d.Inflation, d.InflationCountry = pct/100, c.Name
			}
		}
	}

	in := models.FireInput{Age: d.Age}
	var bad []string
	amount := func(s, name string, def float64) (float64, bool) {
		v, typed, ok := parseFireAmount(s)
		if !ok {
			bad = append(bad, name)
		}
		if !typed {
			return def, false
		}
		return v, true
	}
	var portfolioTyped, spendingTyped, savingTyped bool
	in.Portfolio, portfolioTyped = amount(d.Form.Portfolio, "the portfolio", d.PortfolioDef)
	in.Spending, spendingTyped = amount(d.Form.Spending, "the spending", d.SpendingDef)
	in.PartTime, _ = amount(d.Form.PartTime, "the part-time income", 0)

	// The saving: the income less the spending (both a month).
	income := d.Money.AvgIncome()
	if income > 0 {
		d.SavingFrom = views.FireSavingFromFlow
	} else if claims.Salary.Amount > 0 {
		if rate, ok := visaRate(claims.Salary.Currency, claims.DefaultCurrency.String); ok {
			income, d.SavingFrom = claims.Salary.Amount*rate, views.FireSavingFromSalary
		}
	}
	if d.SavingFrom != "" {
		d.SavingDef = max(income-in.Spending, 0)
	}
	in.Saving, savingTyped = amount(d.Form.Saving, "the saving", d.SavingDef)
	d.FromData = !portfolioTyped || !spendingTyped || !savingTyped

	var ok bool
	if in.Return, ok = parseFirePct(d.Form.Return, fireReturn); !ok {
		bad = append(bad, "the return")
	}
	if in.SWR, ok = parseFirePct(d.Form.SWR, fireSWR); !ok {
		bad = append(bad, "the withdrawal rate")
	}
	if in.Volatility, ok = parseFirePct(d.Form.Vol, fireVol); !ok {
		bad = append(bad, "the volatility")
	}
	in.RetireAge = fireRetireAge
	if s := d.Form.RetireAge; s != "" {
		if in.RetireAge, err = strconv.Atoi(s); err != nil || in.RetireAge <= 0 || in.RetireAge > models.FireLifeAge {
			bad = append(bad, "the retirement age")
		}
	}
	if d.Age <= 0 {
		in.RetireAge = 0
	}
	d.Input = in

	if len(bad) > 0 {
		d.Error = "Check " + strings.Join(bad, ", ") + "."
		return d, nil
	}
	if in.Spending <= 0 && !spendingTyped {
		// Nothing to start from: no spending recorded.
		return d, nil
	}
	if err := in.Validate(); err != nil {
		msg := err.Error()
		d.Error = strings.ToUpper(msg[:1]) + msg[1:] + "."
		return d, nil
	}
	res := models.ComputeFire(in)
	d.Result = &res
	sim := models.SimulateFire(in, models.FireRuns, len(res.Path)-1, models.FireSeed(in))
	d.Sim = &sim
	return d, nil
}
