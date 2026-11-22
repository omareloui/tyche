package handler

import (
	"context"
	"math"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/colguess"
	"github.com/omareloui/tyche/internal/config"
	"github.com/omareloui/tyche/internal/costofliving"
	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/gemini"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/web/views"
)

type fakeColGuess struct {
	calls int
	in    colguess.In
	e     *models.ColCityEstimate
	err   error
}

func (f *fakeColGuess) Estimate(ctx context.Context, in colguess.In) (*models.ColCityEstimate, gemini.Usage, error) {
	f.calls++
	f.in = in
	return f.e, gemini.Usage{InputTokens: 10, OutputTokens: 5}, f.err
}

func (f *fakeColGuess) Model() string { return "fake" }

// colEnv is a tenant spending in a groceries category, with Egypt's and
// Germany's prices and the EGP → EUR rate.
func colEnv(t *testing.T) (*pickEnv, *models.Category) {
	t.Helper()
	e := newPickEnv(t)

	restoreData := costofliving.SetData(map[string]map[string]map[int]float64{
		"EGY": {
			models.ColAverageSeries: {2025: 10},
			models.ColICPTotal:      {costofliving.ICPYear: 2},
			"1101000":               {costofliving.ICPYear: 1},   // food: half the average
			"9060000":               {costofliving.ICPYear: 0.5}, // housing: a quarter
		},
		"DEU": {
			models.ColAverageSeries: {2025: 1},
			models.ColICPTotal:      {costofliving.ICPYear: 1},
			"1101000":               {costofliving.ICPYear: 1},
			"9060000":               {costofliving.ICPYear: 1.5},
		},
	}, nil)
	t.Cleanup(restoreData)
	// Through the dollar: 1 EGP = 0.02 EUR, the seeded account's dollars
	// 50 EGP.
	restoreRates := fxrate.SetRates(map[string]map[string]float64{
		"usd": {"egp": 50, "eur": 1},
	})
	t.Cleanup(restoreRates)

	cat := e.category(t, models.ExpenseTxKind, "")
	e.expense(t, cat)
	return e, cat
}

func TestCostOfLivingCountries(t *testing.T) {
	e, cat := colEnv(t)

	d, err := e.h.colData(e.ctx, url.Values{"home": {"EG"}, "to": {"DE"}, "amount": {"10,000"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Error != "" || d.Result == nil {
		t.Fatalf("no result: %q", d.Error)
	}
	res := d.Result
	if res.AvgRatio != 0.1 || res.Average != 1000 {
		t.Errorf("average = %v (%v)", res.Average, res.AvgRatio)
	}
	if !res.HasBasket || len(res.Groups) != 1 {
		t.Fatalf("basket = %+v", res.Groups)
	}
	group := models.GuessColGroup(cat.Name)
	if res.Groups[0].Group != group {
		t.Errorf("group = %s, want %s", res.Groups[0].Group, group)
	}
	if math.Abs(res.Market-0.02) > 1e-12 || math.Abs(res.PriceLevel-5) > 1e-9 {
		t.Errorf("market %v level %v", res.Market, res.PriceLevel)
	}

	// Picking housing for the category: 0.1 × (1.5 / 0.25) = 0.6 a pound.
	w, r := e.put("/tools/cost-of-living/categories/"+cat.ID, url.Values{
		"col_group": {"housing"}, "home": {"EG"}, "to": {"DE"}, "amount": {"10000"},
	}, map[string]string{"id": cat.ID})
	if _, err := e.h.SetColCategoryGroup(w, r); err != nil {
		t.Fatal(err)
	}
	d, err = e.h.colData(e.ctx, url.Values{"home": {"EG"}, "to": {"DE"}, "amount": {"10000"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if g := d.Result.Groups[0]; g.Group != models.ColHousing || d.Result.Mine < 5999 || d.Result.Mine > 6001 {
		t.Errorf("after picking housing: %+v, mine %v", g, d.Result.Mine)
	}
	if d.Result.Driver != models.ColHousing || d.Result.Diff <= 0 {
		t.Errorf("driver %s diff %v", d.Result.Driver, d.Result.Diff)
	}

	// An unknown group is refused.
	w, r = e.put("/tools/cost-of-living/categories/"+cat.ID, url.Values{"col_group": {"nope"}}, map[string]string{"id": cat.ID})
	if _, err := e.h.SetColCategoryGroup(w, r); statusOf(err) != 422 {
		t.Errorf("bad group: %v", err)
	}
}

func TestCostOfLivingDefaults(t *testing.T) {
	e, _ := colEnv(t)
	getClaims(e.ctx).Timezone = "Europe/Berlin"
	d, err := e.h.colData(e.ctx, url.Values{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if d.Home.ISO2 != "DE" || !d.HomeFromTZ {
		t.Errorf("home = %+v (from tz %v)", d.Home, d.HomeFromTZ)
	}
	if d.Form.Amount == "" || d.AmountFrom != views.ColAmountSpending {
		t.Errorf("amount = %q from %v", d.Form.Amount, d.AmountFrom)
	}
	if d.Result != nil {
		t.Errorf("a result without a target")
	}

	if d, _ := e.h.colData(e.ctx, url.Values{"home": {"EG"}, "to": {"FR"}}, false); !strings.Contains(d.Error, "France") {
		t.Errorf("no data error = %q", d.Error)
	}

	// Without a time zone: the default one's (Cairo), not told as the user's.
	getClaims(e.ctx).Timezone = ""
	if d, _ := e.h.colData(e.ctx, url.Values{}, true); d.Home.ISO2 != "EG" || d.HomeFromTZ {
		t.Errorf("home = %+v (from tz %v)", d.Home, d.HomeFromTZ)
	}
}

func TestCostOfLivingCityCanonical(t *testing.T) {
	e, _ := colEnv(t)
	fake := &fakeColGuess{e: &models.ColCityEstimate{
		HomeCity: "Cairo", TargetCity: "New York City", Overall: 4, Groups: map[models.ColGroup]float64{},
	}}
	e.h.SetColGuess(fake)
	e.h.SetReceiptOCR(nil, config.ReceiptOCR{ColEstimatesPerMonth: 5})
	restore := costofliving.SetData(map[string]map[string]map[int]float64{
		"EGY": {models.ColAverageSeries: {2025: 10}},
		"USA": {models.ColAverageSeries: {2025: 1}},
	}, nil)
	defer restore()
	defer fxrate.SetRates(map[string]map[string]float64{"egp": {"usd": 0.02}})()

	form := url.Values{"home": {"EG"}, "to": {"US"}, "amount": {"1000"}, "mode": {"city"}, "home_city": {"Cairo"}, "to_city": {"NYC"}}
	w, r := e.put("/tools/cost-of-living/city", form, nil)
	r.Method = "POST"
	if _, err := e.h.EstimateCostOfLivingCity(w, r); err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 {
		t.Fatalf("calls = %d", fake.calls)
	}
	// The countries' levels went along: USD 0.1 per EGP at 0.02 = 5.
	if ref := fake.in.Ref; ref == nil || math.Abs(ref.Overall-5) > 1e-9 {
		t.Errorf("reference = %+v", fake.in.Ref)
	}

	// The usual name hits the estimate kept for NYC.
	form.Set("to_city", "new york city")
	d, err := e.h.colData(e.ctx, form, false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Estimate == nil || d.NeedEstimate {
		t.Errorf("the usual name missed the cache")
	}

	// Shown as read: NYC is New York City.
	form.Set("to_city", "NYC")
	d, _ = e.h.colData(e.ctx, form, false)
	if got := views.ColReadAs(d); got != "Estimated for New York City against Cairo." {
		t.Errorf("read as = %q", got)
	}
}

func TestCostOfLivingCity(t *testing.T) {
	e, _ := colEnv(t)
	fake := &fakeColGuess{e: &models.ColCityEstimate{Overall: 4, Groups: map[models.ColGroup]float64{models.ColFood: 2}, Model: "fake"}}
	e.h.SetColGuess(fake)
	e.h.SetReceiptOCR(nil, config.ReceiptOCR{ColEstimatesPerMonth: 1})

	form := url.Values{"home": {"EG"}, "to": {"DE"}, "amount": {"1000"}, "mode": {"city"}, "home_city": {"Cairo"}, "to_city": {"Berlin"}}

	// Nothing asked yet: the result offers the estimate.
	d, err := e.h.colData(e.ctx, form, false)
	if err != nil {
		t.Fatal(err)
	}
	if !d.NeedEstimate || d.EstimatesLeft != 1 || fake.calls != 0 {
		t.Fatalf("need %v left %d calls %d", d.NeedEstimate, d.EstimatesLeft, fake.calls)
	}

	estimate := func() *httptest.ResponseRecorder {
		w, r := e.put("/tools/cost-of-living/city", form, nil)
		r.Method = "POST"
		if _, err := e.h.EstimateCostOfLivingCity(w, r); err != nil {
			t.Fatal(err)
		}
		return w
	}
	// The first answer is the result, not the ask again.
	{
		w, r := e.put("/tools/cost-of-living/city", form, nil)
		r.Method = "POST"
		c, err := e.h.EstimateCostOfLivingCity(w, r)
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		if err := c.Render(e.ctx, &b); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(b.String(), "Estimate with Gemini") || !strings.Contains(b.String(), "Estimated by Gemini") {
			t.Errorf("first answer isn't the result:\n%s", b.String())
		}
	}
	if fake.calls != 1 {
		t.Fatalf("calls = %d", fake.calls)
	}

	// Kept for the month: the live result reads it, asking again is free.
	d, err = e.h.colData(e.ctx, form, false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Estimate == nil || d.Result == nil || d.Result.PriceLevel != 4 {
		t.Fatalf("estimate %+v result %+v", d.Estimate, d.Result)
	}
	estimate()
	if fake.calls != 1 {
		t.Errorf("asked again: %d calls", fake.calls)
	}

	// Typed another way (accents, punctuation): the same estimate.
	form.Set("to_city", " berlin. ")
	if d, _ := e.h.colData(e.ctx, form, false); d.Estimate == nil {
		t.Errorf("berlin. missed the cache")
	}
	form.Set("to_city", "Berlin")

	// Other cities past the month's limit: refused without a call.
	form.Set("to_city", "Munich")
	d, err = e.h.colData(e.ctx, form, false)
	if err != nil {
		t.Fatal(err)
	}
	if d.EstimatesLeft != 0 {
		t.Errorf("left = %d", d.EstimatesLeft)
	}
	w, r := e.put("/tools/cost-of-living/city", form, nil)
	r.Method = "POST"
	if _, err := e.h.EstimateCostOfLivingCity(w, r); err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 {
		t.Errorf("called past the limit")
	}
}
