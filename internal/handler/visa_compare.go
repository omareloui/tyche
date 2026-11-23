package handler

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/countries"
	"github.com/omareloui/tyche/internal/visa"
	"github.com/omareloui/tyche/web/views"
)

// visaCompareDefaults are the visas a new comparison starts with.
var visaCompareDefaults = []views.VisaPickForm{
	{Country: "ES", Visa: "digital-nomad"},
	{Country: "PT", Visa: "d8"},
}

// VisaCompare checks several visas against the user's money side by side.
func (h Handler) VisaCompare(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	b, err := h.repo.GetBalances(ctx)
	if err != nil {
		return nil, err
	}
	d, err := h.visaCompareData(ctx, r.URL.Query())
	if err != nil {
		return nil, err
	}
	return views.VisaCompare(getClaims(ctx), b, d), nil
}

// VisaCompareResult is the comparison again, on every change of its form.
func (h Handler) VisaCompareResult(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	d, err := h.visaCompareData(r.Context(), r.URL.Query())
	if err != nil {
		return nil, err
	}
	w.Header().Set("HX-Replace-Url", views.VisaCompareURL(d))
	return views.VisaCompareBody(d), nil
}

func (h Handler) visaCompareData(ctx context.Context, v url.Values) (*views.VisaCompareData, error) {
	claims := getClaims(ctx)
	d := &views.VisaCompareData{
		Countries: countries.All(),
		Money:     parseVisaMoneyForm(v),
		Default:   claims.DefaultCurrency,
		Best:      -1,
	}
	var forms []views.VisaPickForm
	if v.Get("n") == "" {
		forms = append(forms, visaCompareDefaults...)
	} else {
		n, _ := strconv.Atoi(v.Get("n"))
		n = min(max(n, 2), views.VisaMaxCompare)
		for i := range n {
			forms = append(forms, parseVisaPick(v, "v"+strconv.Itoa(i)+"_"))
		}
	}
	if rm, err := strconv.Atoi(v.Get("remove")); err == nil && rm >= 0 && rm < len(forms) && len(forms) > 2 {
		forms = append(forms[:rm], forms[rm+1:]...)
	}

	mine, err := h.repo.GetVisaRules(ctx)
	if err != nil {
		return nil, err
	}
	d.Data, err = h.repo.GetVisaMoney(ctx, &claims.DefaultCurrency, time.Now())
	if err != nil {
		return nil, err
	}
	for _, f := range forms {
		p, err := h.resolveVisa(ctx, &f, mine)
		if err != nil {
			return nil, err
		}
		h.visaPickResult(ctx, p, d.Data, d.Money, f.Offer, nil)
		d.Visas = append(d.Visas, views.VisaCompareItem{Form: f, Pick: p})
	}
	d.Best = bestVisa(d.Visas)
	return d, nil
}

// bestVisa is the visa the user's money comes closest to (one met first),
// -1 with fewer than two checked.
func bestVisa(items []views.VisaCompareItem) int {
	best, n := -1, 0
	rank := func(res *visa.Result) float64 {
		if res.Verdict != visa.VerdictShort {
			return -1
		}
		return res.Shortest
	}
	for i, it := range items {
		if it.Pick.Result == nil {
			continue
		}
		n++
		if best < 0 || rank(it.Pick.Result) < rank(items[best].Pick.Result) {
			best = i
		}
	}
	if n < 2 {
		return -1
	}
	return best
}
