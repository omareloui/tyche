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
	"github.com/omareloui/tyche/internal/colguess"
	"github.com/omareloui/tyche/internal/costofliving"
	"github.com/omareloui/tyche/internal/countries"
	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/internal/resperr"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

const colEstimateTimeout = 90 * time.Second

// SetCostOfLiving sets the cost of living data's job (status on the admin
// page).
func (h *Handler) SetCostOfLiving(job *costofliving.Job) {
	h.colJob = job
}

// SetColGuess turns the city estimates on (nil keeps them off).
func (h *Handler) SetColGuess(e colguess.Estimator) {
	h.colGuess = e
}

// Tools is the helper calculators' page.
func (h Handler) Tools(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	b, err := h.repo.GetBalances(r.Context())
	if err != nil {
		return nil, err
	}
	return views.Tools(getClaims(r.Context()), b), nil
}

// CostOfLiving is the cost of living calculator, its form filled from the
// query string, else home from the user's time zone and the amount from their
// salary (or monthly spending).
func (h Handler) CostOfLiving(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	b, err := h.repo.GetBalances(ctx)
	if err != nil {
		return nil, err
	}
	d, err := h.colData(ctx, r.URL.Query(), true)
	if err != nil {
		return nil, err
	}
	return views.CostOfLiving(getClaims(ctx), b, d), nil
}

// CostOfLivingResult is the calculator's result, worked out on every change
// of the form (a city estimate only from the cache), keeping the form in
// the page's url.
func (h Handler) CostOfLivingResult(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	d, err := h.colData(r.Context(), r.URL.Query(), false)
	if err != nil {
		return nil, err
	}
	w.Header().Set("HX-Replace-Url", views.CostOfLivingURL(d.Form))
	return views.ColResultSwap(d), nil
}

// EstimateCostOfLivingCity asks the AI for the cities' prices (the month's
// estimate of the same cities is reused), within the user's monthly limit.
func (h Handler) EstimateCostOfLivingCity(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	if h.colGuess == nil {
		return nil, resperr.New(http.StatusNotFound, "City estimates are off")
	}
	if err := r.ParseForm(); err != nil {
		return nil, resperr.New(http.StatusBadRequest, "Invalid form")
	}
	f := parseColForm(r.Form)
	f.Mode = views.ColModeCity

	d, err := h.colData(ctx, colFormValues(f), false)
	if err != nil {
		return nil, err
	}
	w.Header().Set("HX-Replace-Url", views.CostOfLivingURL(d.Form))
	if d.Error != "" || d.Estimate != nil {
		return views.ColResultSwap(d), nil
	}
	if d.Market <= 0 {
		d.Error = "There's no exchange rate between " + d.Home.Currency + " and " + d.Target.Currency + " to compare the cities with."
		return views.ColResultSwap(d), nil
	}

	in := colCityIn(d)
	// The countries' levels as the model's starting point, when there.
	if r, year, _ := countryRatios(d); r != nil {
		in.Ref = colguess.NewReference(r, d.Market, year)
	}
	e, err := h.estimateCity(ctx, in)
	var msg errScan
	if errors.As(err, &msg) {
		d.Error = msg.msg
		return views.ColResultSwap(d), nil
	}
	if err != nil {
		return nil, err
	}

	d.Estimate, d.NeedEstimate = e, false
	h.fillColResult(d)
	return views.ColResultSwap(d), nil
}

// estimateCity asks the AI, within the tenant's monthly estimates, keeping
// the answer for the month.
func (h Handler) estimateCity(ctx context.Context, in colguess.In) (*models.ColCityEstimate, error) {
	l := logger.FromCtx(ctx)
	tenantID, err := repository.TenantIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	left, limit, err := h.repo.AICallsLeft(ctx, tenantID, models.ScanColEstimate, h.ocrCfg.ColEstimatesPerMonth, time.Now())
	if err != nil {
		return nil, err
	}
	if left <= 0 {
		return nil, errScan{"You've had " + strconv.Itoa(limit) + " cities estimated this month, the most you can. It starts again on the 1st."}
	}

	callCtx, cancel := context.WithTimeout(ctx, colEstimateTimeout)
	defer cancel()
	e, usage, callErr := h.colGuess.Estimate(callCtx, in)
	scan := models.ReceiptScan{
		TenantID:      tenantID,
		Kind:          models.ScanColEstimate,
		Model:         h.colGuess.Model(),
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
		l.Error("recording a city estimate", zap.Error(err))
	}
	if errors.Is(callErr, colguess.ErrNoEstimate) {
		l.Info("no city estimate", zap.Error(callErr))
		return nil, errScan{"Couldn't estimate these cities' prices. Check their names, or try again in a bit."}
	}
	if callErr != nil {
		l.Error("estimating a city's prices", zap.Error(callErr))
		return nil, errScan{"Couldn't estimate the prices right now, try again in a bit."}
	}

	// Kept under what was typed and, when the model told them, the cities'
	// usual names, so another way of typing them (NYC, New York) is free.
	now := time.Now()
	keys := []string{in.Key(now)}
	if e.HomeCity != "" && e.TargetCity != "" {
		if k := colguess.Key(e.HomeCity, in.Home.Code, e.TargetCity, in.Target.Code, now); k != keys[0] {
			keys = append(keys, k)
		}
	}
	for _, k := range keys {
		if err := h.repo.SetColCityEstimate(ctx, k, e); err != nil {
			l.Error("keeping a city estimate", zap.Error(err))
		}
	}
	l.Info("estimated a city's prices", zap.Int("groups", len(e.Groups)), zap.Int("sources", len(e.Sources)))
	return e, nil
}

// SetColCategoryGroup picks the price group a category's spending is
// weighed by ("" = guess it from its name), answering the result again.
func (h Handler) SetColCategoryGroup(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		return nil, resperr.New(http.StatusBadRequest, "Invalid form")
	}
	id := r.PathValue("id")
	if _, err := h.repo.GetCategory(ctx, id); err != nil {
		return nil, resperr.New(http.StatusNotFound, "Category not found")
	}
	g := models.ColGroup(r.Form.Get("col_group"))
	if g != "" && !models.ValidColGroup(g) {
		return nil, resperr.New(http.StatusUnprocessableEntity, "Pick a group")
	}
	if err := h.repo.SetCategoryColGroup(ctx, id, g); err != nil {
		return nil, err
	}

	d, err := h.colData(ctx, r.Form, false)
	if err != nil {
		return nil, err
	}
	return views.ColResultSwap(d), nil
}

func parseColForm(v url.Values) views.ColForm {
	f := views.ColForm{
		Home:     strings.ToUpper(strings.TrimSpace(v.Get("home"))),
		To:       strings.ToUpper(strings.TrimSpace(v.Get("to"))),
		Amount:   strings.TrimSpace(v.Get("amount")),
		Mode:     v.Get("mode"),
		HomeCity: strings.Join(strings.Fields(v.Get("home_city")), " "),
		ToCity:   strings.Join(strings.Fields(v.Get("to_city")), " "),
	}
	if f.Mode != views.ColModeCity {
		f.Mode = views.ColModeCountry
	}
	f.HomeCity = truncateRunes(f.HomeCity, 80)
	f.ToCity = truncateRunes(f.ToCity, 80)
	return f
}

func colFormValues(f views.ColForm) url.Values {
	return url.Values{
		"home": {f.Home}, "to": {f.To}, "amount": {f.Amount}, "mode": {f.Mode},
		"home_city": {f.HomeCity}, "to_city": {f.ToCity},
	}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// parseColAmount reads an amount typed with a symbol and separators.
func parseColAmount(s string) (float64, bool) {
	s = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r == '.' {
			return r
		}
		return -1
	}, s)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v <= 0 || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

// colData works the calculator out of the form. defaults fills what's
// missing (a first visit): home from the time zone, the amount from the
// salary or the monthly spending.
func (h Handler) colData(ctx context.Context, v url.Values, defaults bool) (*views.ColData, error) {
	claims := getClaims(ctx)
	d := &views.ColData{
		Form:       parseColForm(v),
		CityOn:     h.colGuess != nil,
		Countries:  countries.All(),
		DataFrom:   time.Now().Year(),
		BasketFrom: repository.ColBasketMonths,
	}
	if data := costofliving.Current(); data != nil {
		d.Countries = d.Countries[:0]
		for _, c := range countries.All() {
			if data.Has(c.ISO3) {
				d.Countries = append(d.Countries, c)
			}
		}
	}

	var ok bool
	if d.Home, ok = countries.ByISO2(d.Form.Home); !ok && (defaults || d.Form.Home == "") {
		tz := ""
		if claims != nil {
			tz = claims.Timezone
		}
		if c, found := countries.FromTimezone(tz); found {
			d.Home, d.HomeFromTZ = c, true
		} else if c, found := countries.FromTimezone(models.DefaultTimezone); found {
			d.Home = c
		}
		d.Form.Home = d.Home.ISO2
	}
	d.Target, _ = countries.ByISO2(d.Form.To)

	basket, err := h.repo.GetColBasket(ctx, d.Home.Currency, time.Now())
	if err != nil {
		return nil, err
	}
	d.Categories, d.BasketMonths, d.BasketMissing = basket.Categories, basket.Months, basket.Missing
	for _, c := range basket.Categories {
		d.MonthlySpend += c.Monthly
	}

	if d.Form.Amount == "" && defaults && d.Home.Currency != "" {
		if claims != nil && claims.Salary.Amount > 0 {
			sal := claims.Salary
			if rate, err := fxrate.GetFxRate(sal.Currency, d.Home.Currency); err == nil {
				d.Form.Amount = strconv.FormatFloat(math.Round(sal.Amount*rate), 'f', -1, 64)
				d.AmountFrom = views.ColAmountSalary
			}
		}
		if d.Form.Amount == "" && d.MonthlySpend > 0 {
			d.Form.Amount = strconv.FormatFloat(math.Round(d.MonthlySpend), 'f', -1, 64)
			d.AmountFrom = views.ColAmountSpending
		}
	}
	d.Amount, _ = parseColAmount(d.Form.Amount)

	if d.Home.ISO2 == "" || d.Target.ISO2 == "" {
		return d, nil
	}
	if rate, err := fxrate.GetFxRate(d.Home.Currency, d.Target.Currency); err == nil {
		d.Market = rate
	}

	if d.Form.Mode == views.ColModeCity {
		if !d.CityOn {
			d.Form.Mode = views.ColModeCountry
		} else {
			d.HomeCity, d.TargetCity = d.Form.HomeCity, d.Form.ToCity
			if d.HomeCity == "" || d.TargetCity == "" {
				return d, nil
			}
			e, err := h.repo.GetColCityEstimate(ctx, colCityIn(d).Key(time.Now()))
			if err != nil {
				return nil, err
			}
			d.Estimate = e
			if e == nil {
				if tenantID, err := repository.TenantIDFromCtx(ctx); err == nil {
					d.EstimatesLeft, _, _ = h.repo.AICallsLeft(ctx, tenantID, models.ScanColEstimate, h.ocrCfg.ColEstimatesPerMonth, time.Now())
				}
				d.NeedEstimate = true
				return d, nil
			}
		}
	}

	h.fillColResult(d)
	return d, nil
}

func colCityIn(d *views.ColData) colguess.In {
	return colguess.In{
		Home:   colguess.City{Name: d.HomeCity, Country: d.Home.Name, Code: d.Home.ISO2, Currency: d.Home.Currency},
		Target: colguess.City{Name: d.TargetCity, Country: d.Target.Name, Code: d.Target.ISO2, Currency: d.Target.Currency},
	}
}

// fillColResult works the result out: from the city estimate when there's
// one, else from the countries' price data.
func (h Handler) fillColResult(d *views.ColData) {
	spend := models.ColSpendByGroup(d.Categories)

	if d.Form.Mode == views.ColModeCity && d.Estimate != nil {
		r := d.Estimate.Ratios(d.Market)
		if r == nil {
			d.Error = "There's no exchange rate between " + d.Home.Currency + " and " + d.Target.Currency + " to compare the cities with."
			return
		}
		res := models.ComputeCol(d.Amount, *r, spend, d.Market)
		d.Result = &res
		return
	}

	r, year, msg := countryRatios(d)
	if r == nil {
		d.Error = msg
		return
	}
	d.DataYear = year
	d.NoGroups = len(r.Groups) == 0
	res := models.ComputeCol(d.Amount, *r, spend, d.Market)
	d.Result = &res
}

// countryRatios are the countries' price ratios (target's currency per
// home's) with the year of their data, else nil and why.
func countryRatios(d *views.ColData) (*models.ColRatios, int, string) {
	data := costofliving.Current()
	now := time.Now().Year()
	home, homeYear, err := data.Prices(d.Home.ISO3, now)
	if err != nil {
		return nil, 0, "There's no price data for " + d.Home.Name + " yet."
	}
	target, targetYear, err := data.Prices(d.Target.ISO3, now)
	if err != nil {
		return nil, 0, "There's no price data for " + d.Target.Name + " yet."
	}
	r := home.Ratios(target)
	if r == nil {
		return nil, 0, "There's no price data for these countries yet."
	}
	return r, min(homeYear, targetYear), ""
}
