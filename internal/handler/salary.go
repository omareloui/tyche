package handler

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/countries"
	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/payroll"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/internal/taxguess"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

const taxGuessTimeout = 90 * time.Second

// defaultWorkHours are the work hours a week saving the salary sets when
// none are set yet.
const defaultWorkHours = 40

// SetTaxGuess turns the AI's tax rules lookups on (nil keeps them off).
func (h *Handler) SetTaxGuess(g taxguess.Guesser) {
	h.taxGuess = g
}

// SalaryCalc is the gross to net salary calculator, its form filled from the
// query string, else the country from the user's time zone and the amount
// from their salary (a take-home pay) on settings.
func (h Handler) SalaryCalc(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	b, err := h.repo.GetBalances(ctx)
	if err != nil {
		return nil, err
	}
	d, err := h.salaryData(ctx, r.URL.Query(), true)
	if err != nil {
		return nil, err
	}
	return views.SalaryCalc(getClaims(ctx), b, d), nil
}

// SalaryCalcBody is the calculator's form and result, when the country or
// the rules change what the form asks.
func (h Handler) SalaryCalcBody(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	d, err := h.salaryData(r.Context(), r.URL.Query(), false)
	if err != nil {
		return nil, err
	}
	w.Header().Set("HX-Replace-Url", views.SalaryURL(d.Form))
	return views.SalaryBody(d), nil
}

// SalaryCalcResult is the calculator's result, on every change of the form,
// keeping the form in the page's url.
func (h Handler) SalaryCalcResult(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	d, err := h.salaryData(r.Context(), r.URL.Query(), false)
	if err != nil {
		return nil, err
	}
	w.Header().Set("HX-Replace-Url", views.SalaryURL(d.Form))
	return views.SalaryResult(d), nil
}

func parseSalaryForm(v url.Values) views.SalaryForm {
	f := views.SalaryForm{
		Country: strings.ToUpper(strings.TrimSpace(v.Get("country"))),
		Rules:   strings.TrimSpace(v.Get("rules")),
		Region:  strings.TrimSpace(v.Get("region")),
		Dir:     v.Get("dir"),
		Amount:  strings.TrimSpace(v.Get("amount")),
		Per:     v.Get("per"),
		Bonus:   strings.TrimSpace(v.Get("bonus")),
		BonusIn: v.Get("bonus_in"),
		Other:   strings.TrimSpace(v.Get("other")),
		Opts:    map[string]string{},
	}
	if len(f.Region) > 80 {
		f.Region = f.Region[:80]
	}
	if f.Dir != views.SalaryDirNet {
		f.Dir = views.SalaryDirGross
	}
	if f.Per != views.SalaryPerYear {
		f.Per = views.SalaryPerMonth
	}
	if f.BonusIn != views.SalaryBonusPct {
		f.BonusIn = views.SalaryBonusAmount
	}
	for k, vs := range v {
		if key, ok := strings.CutPrefix(k, "opt_"); ok && len(vs) > 0 {
			f.Opts[key] = vs[0]
		}
	}
	return f
}

// salaryRulesFor resolves the rules the form picks for the country: one of
// the user's (rules = its id), the AI's kept ones (rules = ai, or the
// country has no built in ones), else the built in ones. ai tells the AI's
// were asked for, none kept yet.
func (h Handler) salaryRulesFor(ctx context.Context, country countries.Country, pick, region string, opts map[string]string) (rules *payroll.Rules, name string, ai bool, err error) {
	switch {
	case pick != "" && pick != views.SalaryRulesBuiltin && pick != views.SalaryRulesAI:
		s, err := h.repo.GetSalaryRule(ctx, pick)
		if err != nil || s == nil {
			return nil, "", false, err
		}
		return &s.Rules, s.Name, false, nil
	case pick == views.SalaryRulesAI || !payroll.HasBuiltin(country.ISO2):
		in := taxguess.In{Country: country.ISO2, Region: region, Year: taxguess.Year(time.Now())}
		rules, err := h.repo.GetTaxRules(ctx, in.Key())
		if err != nil {
			return nil, "", false, err
		}
		return rules, "", rules == nil, nil
	}
	rules, _ = payroll.Builtin(country.ISO2, opts)
	return rules, "", false, nil
}

func (h Handler) salaryData(ctx context.Context, v url.Values, defaults bool) (*views.SalaryData, error) {
	claims := getClaims(ctx)
	d := &views.SalaryData{
		Form:      parseSalaryForm(v),
		Countries: countries.All(),
		AIOn:      h.taxGuess != nil,
	}

	var ok bool
	if d.Country, ok = countries.ByISO2(d.Form.Country); !ok {
		tz := ""
		if claims != nil {
			tz = claims.Timezone
		}
		if c, found := countries.FromTimezone(tz); found {
			d.Country = c
		} else if c, found := countries.FromTimezone(models.DefaultTimezone); found {
			d.Country = c
		}
		d.Form.Country = d.Country.ISO2
	}

	mine, err := h.repo.GetSalaryRules(ctx)
	if err != nil {
		return nil, err
	}
	d.MyRules = mine
	// A rule of the user's picks its country.
	for _, s := range mine {
		if s.ID == d.Form.Rules && s.Country != d.Country.ISO2 {
			if c, ok := countries.ByISO2(s.Country); ok {
				d.Country, d.Form.Country = c, c.ISO2
			}
		}
	}
	d.HasBuiltin = payroll.HasBuiltin(d.Country.ISO2)
	if d.Form.Rules == "" || (d.Form.Rules == views.SalaryRulesBuiltin && !d.HasBuiltin) {
		d.Form.Rules = views.SalaryRulesBuiltin
		if !d.HasBuiltin {
			d.Form.Rules = views.SalaryRulesAI
		}
	}
	if d.Form.Rules == views.SalaryRulesBuiltin {
		d.Options = payroll.BuiltinOptions(d.Country.ISO2)
		d.Form.Opts = payroll.PickOptions(d.Options, d.Form.Opts)
	} else {
		d.Form.Opts = map[string]string{}
	}

	rules, name, needAI, err := h.salaryRulesFor(ctx, d.Country, d.Form.Rules, d.Form.Region, d.Form.Opts)
	if err != nil {
		return nil, err
	}
	if rules == nil && !needAI {
		// A rule of the user's gone: back to the country's.
		d.Form.Rules = ""
		return h.salaryData(ctx, salaryFormValues(d.Form), defaults)
	}
	d.Rules, d.RuleName, d.NeedAI = rules, name, needAI
	if d.Form.Rules == views.SalaryRulesAI || d.NeedAI {
		if tenantID, err := repository.TenantIDFromCtx(ctx); err == nil {
			d.AILeft, d.AILimit, _ = h.repo.AICallsLeft(ctx, tenantID, models.ScanTaxRules, h.ocrCfg.TaxRulesPerMonth, time.Now())
		}
	}

	cur := d.Country.Currency
	if d.Rules != nil {
		cur = d.Rules.Currency
	}
	d.Currency = cur
	// The salary on settings is a take-home pay: what gross it takes.
	if d.Form.Amount == "" && defaults && claims != nil && claims.Salary.Amount > 0 && cur != "" {
		if rate, err := fxrate.GetFxRate(claims.Salary.Currency, cur); err == nil {
			d.Form.Amount = strconv.FormatFloat(math.Round(claims.Salary.Amount*rate), 'f', -1, 64)
			d.Form.Dir, d.Form.Per = views.SalaryDirNet, views.SalaryPerMonth
			d.FromSalary = true
		}
	}

	if d.Rules != nil {
		d.Result, d.Error = computeSalary(d.Rules, d.Form)
	}
	return d, nil
}

// salaryFormValues is the form as a query string.
func salaryFormValues(f views.SalaryForm) url.Values {
	v := url.Values{}
	for k, s := range map[string]string{
		"country": f.Country, "rules": f.Rules, "region": f.Region, "dir": f.Dir,
		"amount": f.Amount, "per": f.Per, "bonus": f.Bonus, "bonus_in": f.BonusIn, "other": f.Other,
	} {
		if s != "" {
			v.Set(k, s)
		}
	}
	for k, s := range f.Opts {
		v.Set("opt_"+k, s)
	}
	return v
}

func parseSalaryAmount(s string) float64 {
	v, _ := parseColAmount(s)
	return v
}

// computeSalary works the form out under the rules, with what's wrong when
// it can't.
func computeSalary(rules *payroll.Rules, f views.SalaryForm) (*payroll.Result, string) {
	amount := parseSalaryAmount(f.Amount)
	if amount <= 0 {
		return nil, ""
	}
	other := parseSalaryAmount(f.Other) / 100
	if other >= 1 {
		return nil, "Other tax has to be under 100%."
	}
	in := payroll.Input{OtherRate: other}
	if f.BonusIn == views.SalaryBonusPct {
		in.BonusRate = parseSalaryAmount(f.Bonus) / 100
		if in.BonusRate > 10 {
			return nil, "A bonus over 1,000% of the gross? Check it."
		}
	} else {
		in.Bonus = parseSalaryAmount(f.Bonus)
	}
	annual := amount
	if f.Per == views.SalaryPerMonth {
		annual = amount * float64(rules.Pays())
	}
	if f.Dir == views.SalaryDirNet {
		g, ok := rules.GrossFor(annual, in)
		if !ok {
			return nil, "No gross pay takes that much home under these rules."
		}
		annual = g
	}
	in.AnnualGross = annual
	res := rules.Compute(in)
	return &res, ""
}

// errTax is a lookup's problem to tell the user.
type errTax struct{ msg string }

func (e errTax) Error() string { return e.msg }

// FetchSalaryRules has the AI look up the tax rules of the form's country
// (region), within the user's monthly lookups: kept ones are reused, unless
// refresh asks for them again (counted). Answers the calculator's result, or
// the comparison's when view=compare.
func (h Handler) FetchSalaryRules(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	if h.taxGuess == nil {
		return nil, resperr.New(http.StatusNotFound, "Tax rules lookups are off")
	}
	if err := r.ParseForm(); err != nil {
		return nil, resperr.New(http.StatusBadRequest, "Invalid form")
	}
	compare := r.Form.Get("view") == "compare"
	country, region := r.Form.Get("country"), r.Form.Get("region")
	if compare {
		i := r.Form.Get("fetch")
		country, region = r.Form.Get("o"+i+"_country"), r.Form.Get("o"+i+"_region")
	}
	c, ok := countries.ByISO2(strings.ToUpper(strings.TrimSpace(country)))
	if !ok {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Pick a country")
	}
	region = strings.TrimSpace(region)
	if len(region) > 80 {
		region = region[:80]
	}
	in := taxguess.In{Country: c.ISO2, Name: c.Name, Currency: c.Currency, Region: region, Year: taxguess.Year(time.Now())}

	var msg string
	cached, err := h.repo.GetTaxRules(ctx, in.Key())
	if err != nil {
		return nil, err
	}
	if cached == nil || r.Form.Get("refresh") != "" {
		_, err := h.lookUpTaxRules(ctx, in)
		var e errTax
		if errors.As(err, &e) {
			msg = e.msg
		} else if err != nil {
			return nil, err
		}
	}

	if compare {
		d, err := h.salaryCompareData(ctx, r.Form)
		if err != nil {
			return nil, err
		}
		d.Error = msg
		w.Header().Set("HX-Replace-Url", views.SalaryCompareURL(d))
		return views.SalaryCompareBody(d), nil
	}
	d, err := h.salaryData(ctx, r.Form, false)
	if err != nil {
		return nil, err
	}
	if msg != "" {
		d.Error = msg
	}
	w.Header().Set("HX-Replace-Url", views.SalaryURL(d.Form))
	return views.SalaryResult(d), nil
}

// lookUpTaxRules asks the AI, within the tenant's monthly lookups, keeping
// the answer for everyone.
func (h Handler) lookUpTaxRules(ctx context.Context, in taxguess.In) (*payroll.Rules, error) {
	l := logger.FromCtx(ctx)
	tenantID, err := repository.TenantIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	left, limit, err := h.repo.AICallsLeft(ctx, tenantID, models.ScanTaxRules, h.ocrCfg.TaxRulesPerMonth, time.Now())
	if err != nil {
		return nil, err
	}
	if left <= 0 {
		return nil, errTax{"You've had tax rules looked up " + strconv.Itoa(limit) + " times this month, the most you can. It starts again on the 1st."}
	}

	callCtx, cancel := context.WithTimeout(ctx, taxGuessTimeout)
	defer cancel()
	rules, usage, callErr := h.taxGuess.Guess(callCtx, in)
	scan := models.ReceiptScan{
		TenantID:      tenantID,
		Kind:          models.ScanTaxRules,
		Model:         h.taxGuess.Model(),
		InputTokens:   usage.InputTokens,
		OutputTokens:  usage.OutputTokens,
		SearchQueries: usage.SearchQueries,
		CostMicros:    usage.CostMicros(h.ocrCfg.Prices()),
		OK:            callErr == nil,
	}
	if callErr != nil {
		scan.Error = callErr.Error()
	}
	if err := h.repo.RecordReceiptScan(ctx, scan); err != nil {
		l.Error("recording a tax rules lookup", zap.Error(err))
	}
	if errors.Is(callErr, taxguess.ErrNoRules) {
		l.Info("no tax rules", zap.Error(callErr))
		return nil, errTax{"Couldn't find this place's tax rules. Check the region's name, or type the rules yourself."}
	}
	if callErr != nil {
		l.Error("looking up tax rules", zap.Error(callErr))
		return nil, errTax{"Couldn't look the tax rules up right now, try again in a bit."}
	}
	if err := h.repo.SetTaxRules(ctx, in.Key(), h.taxGuess.Model(), rules); err != nil {
		l.Error("keeping tax rules", zap.Error(err))
	}
	l.Info("looked up tax rules", zap.Int("brackets", len(rules.Brackets)), zap.Int("contributions", len(rules.Contributions)))
	return rules, nil
}

// SaveSalaryNet sets the salary on settings to the calculator's take-home
// pay a month (the bonus left out), in the rules' currency, keeping the work
// hours.
func (h *Handler) SaveSalaryNet(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		return nil, resperr.New(http.StatusBadRequest, "Invalid form")
	}
	d, err := h.salaryData(ctx, r.Form, false)
	if err != nil {
		return nil, err
	}
	if d.Result == nil || d.Rules == nil {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Work a salary out first")
	}
	curs, err := h.repo.GetCurrencies(ctx)
	if err != nil {
		return nil, err
	}
	var cur *models.Currency
	for i := range curs {
		if curs[i].String == d.Rules.Currency {
			cur = &curs[i]
		}
	}
	if cur == nil {
		return nil, resperr.New(http.StatusUnprocessableEntity, d.Rules.Currency+" isn't one of your currencies")
	}
	ws, err := h.repo.GetWorkSettings(ctx)
	if err != nil {
		return nil, err
	}
	hours := ws.HoursPerWeek
	if hours <= 0 {
		hours = defaultWorkHours
	}
	monthly := cur.Round((d.Result.Net - d.Result.BonusNet) / 12)
	if err := h.repo.SetWorkSettings(ctx, &models.WorkSettings{MonthlySalary: monthly, Currency: *cur, HoursPerWeek: hours}); err != nil {
		return nil, err
	}
	// The salary rides in the claims.
	if err := h.refreshClaims(w, ctx); err != nil {
		return nil, err
	}
	logger.FromCtx(ctx).Info("saved the salary calculator's net as the salary")
	return views.SalarySaved(cur.Format(monthly)), nil
}
