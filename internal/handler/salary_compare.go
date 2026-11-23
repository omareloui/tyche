package handler

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/costofliving"
	"github.com/omareloui/tyche/internal/countries"
	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/payroll"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/web/views"
)

// SalaryCompare compares job offers of different countries: each one's
// take-home pay, in the default currency and in what it buys at home.
func (h Handler) SalaryCompare(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	b, err := h.repo.GetBalances(ctx)
	if err != nil {
		return nil, err
	}
	d, err := h.salaryCompareData(ctx, r.URL.Query())
	if err != nil {
		return nil, err
	}
	return views.SalaryCompare(getClaims(ctx), b, d), nil
}

// SalaryCompareResult is the comparison again, on every change of its form.
func (h Handler) SalaryCompareResult(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	d, err := h.salaryCompareData(r.Context(), r.URL.Query())
	if err != nil {
		return nil, err
	}
	w.Header().Set("HX-Replace-Url", views.SalaryCompareURL(d))
	return views.SalaryCompareBody(d), nil
}

func parseOfferForm(v url.Values, i int) views.SalaryOfferForm {
	p := "o" + strconv.Itoa(i) + "_"
	f := views.SalaryOfferForm{
		Country: strings.ToUpper(strings.TrimSpace(v.Get(p + "country"))),
		Rules:   strings.TrimSpace(v.Get(p + "rules")),
		Region:  strings.TrimSpace(v.Get(p + "region")),
		Amount:  strings.TrimSpace(v.Get(p + "amount")),
		Dir:     v.Get(p + "dir"),
		Per:     v.Get(p + "per"),
		Bonus:   strings.TrimSpace(v.Get(p + "bonus")),
		BonusIn: v.Get(p + "bonus_in"),
		Other:   strings.TrimSpace(v.Get(p + "other")),
		Opts:    map[string]string{},
	}
	if f.BonusIn != views.SalaryBonusPct {
		f.BonusIn = views.SalaryBonusAmount
	}
	if f.Dir != views.SalaryDirNet {
		f.Dir = views.SalaryDirGross
	}
	if f.Per != views.SalaryPerYear {
		f.Per = views.SalaryPerMonth
	}
	if len(f.Region) > 80 {
		f.Region = f.Region[:80]
	}
	for k, vs := range v {
		if key, ok := strings.CutPrefix(k, p+"opt_"); ok && len(vs) > 0 {
			f.Opts[key] = vs[0]
		}
	}
	return f
}

func (h Handler) salaryCompareData(ctx context.Context, v url.Values) (*views.SalaryCompareData, error) {
	claims := getClaims(ctx)
	d := &views.SalaryCompareData{
		Countries: countries.All(),
		AIOn:      h.taxGuess != nil,
		Best:      -1,
	}
	if claims != nil {
		d.DefaultCurrency = claims.DefaultCurrency.String
	}
	tz := models.DefaultTimezone
	if claims != nil && claims.Timezone != "" {
		tz = claims.Timezone
	}
	d.Home, _ = countries.FromTimezone(tz)

	n, _ := strconv.Atoi(v.Get("n"))
	n = min(max(n, 2), views.SalaryMaxOffers)
	var forms []views.SalaryOfferForm
	for i := range n {
		forms = append(forms, parseOfferForm(v, i))
	}
	if rm, err := strconv.Atoi(v.Get("remove")); err == nil && rm >= 0 && rm < len(forms) && len(forms) > 2 {
		forms = append(forms[:rm], forms[rm+1:]...)
	}
	// New offers start from home.
	for i := range forms {
		if forms[i].Country == "" {
			forms[i].Country = d.Home.ISO2
		}
	}

	mine, err := h.repo.GetSalaryRules(ctx)
	if err != nil {
		return nil, err
	}
	d.MyRules = mine
	if tenantID, err := repository.TenantIDFromCtx(ctx); err == nil && d.AIOn {
		d.AILeft, _, _ = h.repo.AICallsLeft(ctx, tenantID, models.ScanTaxRules, h.ocrCfg.TaxRulesPerMonth, time.Now())
	}

	var homePrices *models.ColPrices
	homeYear := 0
	data := costofliving.Current()
	if data != nil && d.Home.ISO3 != "" {
		if p, y, err := data.Prices(d.Home.ISO3, time.Now().Year()); err == nil {
			homePrices, homeYear = &p, y
		}
	}

	for _, f := range forms {
		o := views.SalaryOffer{Form: f}
		o.Country, _ = countries.ByISO2(f.Country)
		o.HasBuiltin = payroll.HasBuiltin(o.Country.ISO2)
		if o.Form.Rules == "" || (o.Form.Rules == views.SalaryRulesBuiltin && !o.HasBuiltin) {
			o.Form.Rules = views.SalaryRulesBuiltin
			if !o.HasBuiltin {
				o.Form.Rules = views.SalaryRulesAI
			}
		}
		if o.Form.Rules == views.SalaryRulesBuiltin {
			o.Options = payroll.BuiltinOptions(o.Country.ISO2)
			o.Form.Opts = payroll.PickOptions(o.Options, o.Form.Opts)
		} else {
			o.Form.Opts = map[string]string{}
		}
		if o.Country.ISO2 != "" {
			rules, name, needAI, err := h.salaryRulesFor(ctx, o.Country, o.Form.Rules, o.Form.Region, o.Form.Opts)
			if err != nil {
				return nil, err
			}
			if rules == nil && !needAI {
				o.Form.Rules = views.SalaryRulesBuiltin
				if !o.HasBuiltin {
					o.Form.Rules = views.SalaryRulesAI
				}
				rules, name, needAI, err = h.salaryRulesFor(ctx, o.Country, o.Form.Rules, o.Form.Region, o.Form.Opts)
				if err != nil {
					return nil, err
				}
			}
			o.Rules, o.RuleName, o.NeedAI = rules, name, needAI
		}
		o.Currency = o.Country.Currency
		if o.Rules != nil {
			o.Currency = o.Rules.Currency
			o.Result, o.Error = computeSalary(o.Rules, views.SalaryForm{
				Amount: o.Form.Amount, Per: o.Form.Per, Dir: o.Form.Dir,
				Bonus: o.Form.Bonus, BonusIn: o.Form.BonusIn, Other: o.Form.Other,
			})
		}
		if o.Result != nil {
			monthly := o.Result.Net / 12
			if d.DefaultCurrency != "" {
				if rate, err := fxrate.GetFxRate(o.Currency, d.DefaultCurrency); err == nil {
					o.NetDefault, o.RateDefault = monthly*rate, rate
				}
			}
			if homePrices != nil && o.Country.ISO3 != "" {
				if p, y, err := data.Prices(o.Country.ISO3, time.Now().Year()); err == nil {
					// Target's currency per home's at the price levels.
					if r := homePrices.Ratios(p); r != nil && r.Avg > 0 {
						o.NetAtHome, o.PriceRatio, o.PriceYear = monthly/r.Avg, r.Avg, min(y, homeYear)
						if rate, err := fxrate.GetFxRate(d.Home.Currency, o.Currency); err == nil {
							o.MarketRatio = rate
						}
					}
				}
			}
		}
		d.Offers = append(d.Offers, o)
	}
	d.Best = bestOffer(d.Offers)
	return d, nil
}

// bestOffer is the offer taking home the most, by what it buys at home when
// every offer worked out has it, else in the default currency; -1 with
// fewer than two to compare.
func bestOffer(offers []views.SalaryOffer) int {
	atHome, n := true, 0
	for _, o := range offers {
		if o.Result != nil {
			n++
			atHome = atHome && o.NetAtHome > 0
		}
	}
	if n < 2 {
		return -1
	}
	best, bestVal := -1, 0.0
	for i, o := range offers {
		val := o.NetDefault
		if atHome {
			val = o.NetAtHome
		}
		if o.Result != nil && val > bestVal {
			best, bestVal = i, val
		}
	}
	return best
}
