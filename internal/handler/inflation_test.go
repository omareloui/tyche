package handler

import (
	"math"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/costofliving"
	"github.com/omareloui/tyche/web/views"
)

func TestInflationCalc(t *testing.T) {
	e := newPickEnv(t)
	restore := costofliving.SetData(nil, map[string]map[int]float64{
		"EGY": {2020: 5, 2021: 10, 2022: 20, 2023: 30},
		"DEU": {2023: 6},
	})
	t.Cleanup(restore)
	now := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)

	d := inflationData(e.ctx, url.Values{"country": {"eg"}, "amount": {"1,000"}, "from": {"2020"}, "to": {"2023"}}, now, false)
	if d.Error != "" || d.Result == nil {
		t.Fatalf("no result: %q", d.Error)
	}
	if want := 1000 * 1.1 * 1.2 * 1.3; math.Abs(d.Result.Worth-want) > 1e-9 {
		t.Errorf("worth = %v, want %v", d.Result.Worth, want)
	}
	if len(d.Countries) != 2 || d.Years[0] != 2023 || d.Estimated != 2023 {
		t.Errorf("countries %d, years from %d, estimated %d", len(d.Countries), d.Years[0], d.Estimated)
	}
	if u := views.InflationURL(d.Form); u != "/tools/inflation?amount=1%2C000&country=EG&from=2020&to=2023" {
		t.Errorf("url = %s", u)
	}

	// Germany's data go back to 2022 (its 2023 change starts then).
	if d := inflationData(e.ctx, url.Values{"country": {"DE"}, "amount": {"100"}, "from": {"2022"}, "to": {"2023"}}, now, false); d.Result == nil {
		t.Errorf("2022 → 2023: %q", d.Error)
	}
	if d := inflationData(e.ctx, url.Values{"country": {"DE"}, "amount": {"100"}, "from": {"2021"}, "to": {"2023"}}, now, false); !strings.Contains(d.Error, "2022") {
		t.Errorf("before the data: %q", d.Error)
	}
	if d := inflationData(e.ctx, url.Values{"country": {"EG"}, "amount": {"100"}, "from": {"2020"}, "to": {"2024"}}, now, false); d.Error == "" {
		t.Error("a year ahead accepted")
	}
	if d := inflationData(e.ctx, url.Values{"country": {"FR"}, "amount": {"100"}, "from": {"2020"}, "to": {"2023"}}, now, false); !strings.Contains(d.Error, "France") {
		t.Errorf("no data error = %q", d.Error)
	}

	// Defaults: the time zone's country, ten years back to this one.
	getClaims(e.ctx).Timezone = "Europe/Berlin"
	d = inflationData(e.ctx, url.Values{}, now, true)
	if d.Country.ISO2 != "DE" || !d.CountryFromTZ || d.Form.From != "2013" || d.Form.To != "2023" || d.Form.Amount == "" {
		t.Errorf("defaults = %+v", d.Form)
	}

	// Nothing loaded yet.
	costofliving.SetData(nil, nil)
	if d := inflationData(e.ctx, url.Values{}, now, true); !d.NoData {
		t.Error("no data not told")
	}
}
