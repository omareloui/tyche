package costofliving

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/shareddb"
)

func newTestJob(t *testing.T, srv *httptest.Server) *Job {
	t.Helper()
	dir := t.TempDir()
	// The migrations are read from the repo's root.
	t.Chdir("../..")
	db, q, err := shareddb.Open(filepath.Join(dir, "shared.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	j := NewJob(db, q, time.Hour)
	if srv == nil {
		return j
	}
	j.urls = sourceURLs{
		avg:       srv.URL + "/avg?page=%d",
		icp:       srv.URL + "/icp/%s?page=%d",
		inflation: srv.URL + "/imf",
	}
	return j
}

const avgPage1 = `[{"page":1,"pages":2,"per_page":"2","total":4},[
 {"countryiso3code":"EGY","date":"2024","value":4},
 {"countryiso3code":"EGY","date":"2023","value":3},
 {"countryiso3code":"AFE","date":"2024","value":9}]]`

const avgPage2 = `[{"page":2,"pages":2,"per_page":"2","total":4},[
 {"countryiso3code":"DEU","date":"2024","value":0.7},
 {"countryiso3code":"DEU","date":"2025","value":null}]]`

func icpEntry(country, series string, v string) string {
	return fmt.Sprintf(`{"variable":[{"concept":"Country","id":"%s"},{"concept":"Series","id":"%s"}],"value":%s}`, country, series, v)
}

func TestRunOnce(t *testing.T) {
	icp := `{"page":1,"pages":1,"source":{"data":[` + strings.Join([]string{
		icpEntry("EGY", models.ColICPTotal, "2"),
		icpEntry("EGY", "9060000", "1"), // housing half the average
		icpEntry("EGY", "1101000", "null"),
		icpEntry("DEU", models.ColICPTotal, "0.8"),
		icpEntry("DEU", "9060000", "0.8"),
		icpEntry("USA", models.ColICPTotal, "1"),
	}, ",") + `]}}`
	imf := `{"values":{"PCPIPCH":{"EGY":{"2024":28,"2025":20,"2026":10,"1990":5},"USA":{"2025":2,"2026":2},"DEU":{"2025":2}}}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/avg" && r.URL.Query().Get("page") == "1":
			fmt.Fprint(w, avgPage1)
		case r.URL.Path == "/avg":
			fmt.Fprint(w, avgPage2)
		case strings.HasPrefix(r.URL.Path, "/icp/"):
			fmt.Fprint(w, icp)
		case r.URL.Path == "/imf":
			fmt.Fprint(w, imf)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	restore := SetData(nil, nil)
	defer restore()

	j := newTestJob(t, srv)
	if err := j.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if j.FetchedAt(context.Background()).IsZero() {
		t.Errorf("fetched at not kept")
	}

	d := Current()
	egy, year, err := d.Prices("EGY", 2026)
	if err != nil || year != 2024 {
		t.Fatalf("EGY = %v %d %v", egy, year, err)
	}
	// 4 rolled by 2025 (1.2 / 1.02) and 2026 (1.1 / 1.02).
	want := 4 * 1.2 / 1.02 * 1.1 / 1.02
	if math.Abs(egy.Level-want) > 1e-9 {
		t.Errorf("EGY level = %v, want %v", egy.Level, want)
	}
	if egy.Rel[models.ColHousing] != 0.5 || egy.Rel[models.ColFood] != 0 {
		t.Errorf("EGY rel = %v", egy.Rel)
	}

	deu, year, err := d.Prices("DEU", 2026)
	if err != nil || year != 2024 {
		t.Fatalf("DEU = %v %d %v", deu, year, err)
	}
	// 2026 has no German inflation: only 2025 rolls.
	if math.Abs(deu.Level-0.7) > 1e-9 {
		t.Errorf("DEU level = %v", deu.Level)
	}

	// USA only has the ICP total: the level is it.
	if usa, year, err := d.Prices("USA", 2021); err != nil || year != ICPYear || usa.Level != 1 {
		t.Errorf("USA = %v %d %v", usa, year, err)
	}
	if _, _, err := d.Prices("FRA", 2026); err != ErrNoData {
		t.Errorf("FRA err = %v", err)
	}
	if st := j.Status(); st.Countries != 3 || st.LastErr != "" {
		t.Errorf("status = %+v", st)
	}

	// Loading again from the db gives the same.
	SetData(nil, nil)
	if err := j.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if again, _, _ := Current().Prices("EGY", 2026); again.Level != egy.Level {
		t.Errorf("reloaded EGY = %v", again.Level)
	}
}

func TestRunOnceAllFail(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	j := newTestJob(t, srv)
	if err := j.RunOnce(context.Background()); err == nil {
		t.Fatal("want an error")
	}
	if j.Status().LastErr == "" {
		t.Error("last error not kept")
	}
}
