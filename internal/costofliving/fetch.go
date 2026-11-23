package costofliving

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/countries"
	"github.com/omareloui/tyche/internal/models"
)

const (
	fetchTimeout = 90 * time.Second
	// maxBody caps a source's answer.
	maxBody = 32 << 20
	// inflationSince is the first year of inflation kept: the IMF's series
	// start then, read back by the inflation calculator.
	inflationSince = 1980
)

type sourceURLs struct {
	// avg is the World Bank's household consumption PPP, %d the page.
	avg string
	// icp is the ICP 2021 groups' PPPs, %s the series, %d the page.
	icp string
	// inflation is the IMF's consumer prices' change.
	inflation string
}

var defaultURLs = sourceURLs{
	avg:       "https://api.worldbank.org/v2/country/all/indicator/PA.NUS.PRVT.PP?format=json&mrv=6&per_page=5000&page=%d",
	icp:       "https://api.worldbank.org/v2/sources/90/country/all/series/%s/classification/PPPGlob/time/YR2021/data?format=json&per_page=5000&page=%d",
	inflation: "https://www.imf.org/external/datamapper/api/v1/PCPIPCH",
}

type pppRow struct {
	country, series string
	year            int
	value           float64
}

type inflationRow struct {
	country string
	year    int
	pct     float64
}

type fetched struct {
	ppp       []pppRow
	inflation []inflationRow
	errs      []error
}

func (j *Job) fetch(ctx context.Context) fetched {
	var f fetched

	if rows, err := j.fetchAvg(ctx); err != nil {
		f.errs = append(f.errs, fmt.Errorf("world bank ppp: %w", err))
	} else {
		f.ppp = append(f.ppp, rows...)
	}
	if rows, err := j.fetchICP(ctx); err != nil {
		f.errs = append(f.errs, fmt.Errorf("icp 2021: %w", err))
	} else {
		f.ppp = append(f.ppp, rows...)
	}
	if rows, err := j.fetchInflation(ctx); err != nil {
		f.errs = append(f.errs, fmt.Errorf("imf inflation: %w", err))
	} else {
		f.inflation = rows
	}
	return f
}

// flexInt reads a number sent as a number or a string.
type flexInt int

func (n *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*n = 0
		return nil
	}
	v, err := strconv.Atoi(s)
	*n = flexInt(v)
	return err
}

type wbPage struct {
	Page  flexInt `json:"page"`
	Pages flexInt `json:"pages"`
}

func (j *Job) fetchAvg(ctx context.Context) ([]pppRow, error) {
	var out []pppRow
	for page := 1; ; page++ {
		var body []json.RawMessage
		if err := j.getJSON(ctx, fmt.Sprintf(j.urls.avg, page), &body); err != nil {
			return nil, err
		}
		if len(body) < 2 {
			return nil, fmt.Errorf("unexpected answer: %d parts", len(body))
		}
		var meta wbPage
		if err := json.Unmarshal(body[0], &meta); err != nil {
			return nil, err
		}
		var rows []struct {
			Country string   `json:"countryiso3code"`
			Date    string   `json:"date"`
			Value   *float64 `json:"value"`
		}
		if err := json.Unmarshal(body[1], &rows); err != nil {
			return nil, err
		}
		for _, r := range rows {
			year, err := strconv.Atoi(r.Date)
			if err != nil || r.Value == nil || *r.Value <= 0 {
				continue
			}
			if _, ok := countries.ByISO3(r.Country); !ok {
				continue
			}
			out = append(out, pppRow{country: r.Country, series: models.ColAverageSeries, year: year, value: *r.Value})
		}
		if int(meta.Page) >= int(meta.Pages) {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no data")
	}
	return out, nil
}

func (j *Job) fetchICP(ctx context.Context) ([]pppRow, error) {
	codes := []string{models.ColICPTotal}
	for _, g := range models.ColGroups {
		codes = append(codes, g.ICP)
	}
	series := strings.Join(codes, ";")

	var out []pppRow
	for page := 1; ; page++ {
		var body struct {
			wbPage
			Source struct {
				Data []struct {
					Variable []struct {
						Concept string `json:"concept"`
						ID      string `json:"id"`
					} `json:"variable"`
					Value *float64 `json:"value"`
				} `json:"data"`
			} `json:"source"`
		}
		if err := j.getJSON(ctx, fmt.Sprintf(j.urls.icp, series, page), &body); err != nil {
			return nil, err
		}
		for _, d := range body.Source.Data {
			if d.Value == nil || *d.Value <= 0 {
				continue
			}
			var country, code string
			for _, v := range d.Variable {
				switch v.Concept {
				case "Country":
					country = v.ID
				case "Series":
					code = v.ID
				}
			}
			if _, ok := countries.ByISO3(country); !ok || code == "" {
				continue
			}
			out = append(out, pppRow{country: country, series: code, year: ICPYear, value: *d.Value})
		}
		if int(body.Page) >= int(body.Pages) {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no data")
	}
	return out, nil
}

func (j *Job) fetchInflation(ctx context.Context) ([]inflationRow, error) {
	var body struct {
		Values struct {
			PCPIPCH map[string]map[string]*float64 `json:"PCPIPCH"`
		} `json:"values"`
	}
	if err := j.getJSON(ctx, j.urls.inflation, &body); err != nil {
		return nil, err
	}
	var out []inflationRow
	for country, years := range body.Values.PCPIPCH {
		if _, ok := countries.ByISO3(country); !ok {
			continue
		}
		for y, v := range years {
			year, err := strconv.Atoi(y)
			if err != nil || v == nil || year < inflationSince {
				continue
			}
			out = append(out, inflationRow{country: country, year: year, pct: *v})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no data")
	}
	return out, nil
}

func (j *Job) getJSON(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := j.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: unexpected status %d", url, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("%s: %w", url, err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", url, err)
	}
	return nil
}
