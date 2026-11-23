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
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/web/views"
)

const (
	// inflationBackYears is how far back the from year starts.
	inflationBackYears = 10
	inflationAmount    = "1,000"
	// inflationFirstYear is the first year offered (the IMF's series').
	inflationFirstYear = 1980
)

// InflationCalc is the inflation calculator, its form filled from the query
// string, else the time zone's country and ten years back.
func (h Handler) InflationCalc(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	b, err := h.repo.GetBalances(ctx)
	if err != nil {
		return nil, err
	}
	return views.InflationCalc(getClaims(ctx), b, inflationData(ctx, r.URL.Query(), time.Now(), true)), nil
}

// InflationCalcResult is the calculator's result, worked out on every change
// of the form, keeping the form in the page's url.
func (h Handler) InflationCalcResult(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	d := inflationData(r.Context(), r.URL.Query(), time.Now(), false)
	w.Header().Set("HX-Replace-Url", views.InflationURL(d.Form))
	return views.InflationResultBody(d), nil
}

func parseInflationYear(s string) int {
	y, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return y
}

// inflationData works the calculator out of the form. defaults fills what's
// missing (a first visit).
func inflationData(ctx context.Context, v url.Values, now time.Time, defaults bool) *views.InflationData {
	thisYear := now.Year()
	d := &views.InflationData{
		Form: views.InflationForm{
			Country: strings.ToUpper(strings.TrimSpace(v.Get("country"))),
			Amount:  strings.TrimSpace(v.Get("amount")),
			From:    strings.TrimSpace(v.Get("from")),
			To:      strings.TrimSpace(v.Get("to")),
		},
		Estimated: thisYear,
	}
	for y := thisYear; y >= inflationFirstYear; y-- {
		d.Years = append(d.Years, y)
	}

	data := costofliving.Current()
	for _, c := range countries.All() {
		if len(data.Inflation(c.ISO3)) > 0 {
			d.Countries = append(d.Countries, c)
		}
	}
	if len(d.Countries) == 0 {
		d.NoData = true
	}

	var ok bool
	if d.Country, ok = countries.ByISO2(d.Form.Country); !ok && (defaults || d.Form.Country == "") {
		tz := ""
		if claims := getClaims(ctx); claims != nil {
			tz = claims.Timezone
		}
		if c, found := countries.FromTimezone(tz); found {
			d.Country, d.CountryFromTZ = c, true
		} else if c, found := countries.FromTimezone(models.DefaultTimezone); found {
			d.Country = c
		}
		d.Form.Country = d.Country.ISO2
	}
	if defaults {
		if d.Form.Amount == "" {
			d.Form.Amount = inflationAmount
		}
		if d.Form.From == "" {
			d.Form.From = strconv.Itoa(thisYear - inflationBackYears)
		}
		if d.Form.To == "" {
			d.Form.To = strconv.Itoa(thisYear)
		}
	}
	if d.NoData || d.Country.ISO2 == "" {
		return d
	}

	amount, ok := parseColAmount(d.Form.Amount)
	from, to := parseInflationYear(d.Form.From), parseInflationYear(d.Form.To)
	if !ok || from == 0 || to == 0 {
		return d
	}
	d.Amount = amount

	rates := data.Inflation(d.Country.ISO3)
	first, _ := data.InflationYears(d.Country.ISO3)
	switch {
	case len(rates) == 0:
		d.Error = "There's no inflation data for " + d.Country.Name + "."
		return d
	case from < inflationFirstYear || to < inflationFirstYear || from > thisYear || to > thisYear:
		d.Error = "Pick years from " + strconv.Itoa(inflationFirstYear) + " to " + strconv.Itoa(thisYear) + "."
		return d
	case min(from, to) < first-1:
		// The first year's change starts from the year before it.
		d.Error = d.Country.Name + "'s inflation data go back to " + strconv.Itoa(first-1) + "."
		return d
	}
	res, err := models.ComputeInflation(rates, amount, from, to)
	if err != nil {
		d.Error = "Some years between " + strconv.Itoa(min(from, to)) + " and " + strconv.Itoa(max(from, to)) + " have no inflation data for " + d.Country.Name + "."
		return d
	}
	d.Result = &res
	return d
}
