// Package costofliving keeps the price levels of the countries in memory for
// the cost of living calculator. A Job fetches them (World Bank household
// consumption PPPs, ICP 2021 group PPPs, IMF inflation) periodically into
// the shared database and swaps the in-memory data when done.
package costofliving

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/omareloui/tyche/internal/countries"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/sharedqueries"
	"go.uber.org/zap"
)

// ErrNoData is a country without price data.
var ErrNoData = errors.New("no price data for the country")

// ICPYear is the ICP round the groups' prices come from.
const ICPYear = 2021

// Data are the stored price data.
type Data struct {
	// ppp are country (ISO3) → series → year → PPP.
	ppp map[string]map[string]map[int]float64
	// inflation are country (ISO3) → year → consumer prices' change (%).
	inflation map[string]map[int]float64
}

// usISO3 is the country the PPPs are relative to (international $ = US $).
const usISO3 = "USA"

// Prices are a country's prices in the year now: the latest household
// consumption PPP rolled to now by its inflation against the US', with the
// ICP groups' prices relative to the whole. Year is the PPP's own year.
func (d *Data) Prices(iso3 string, now int) (p models.ColPrices, year int, err error) {
	if d == nil {
		return p, 0, ErrNoData
	}
	series := d.ppp[iso3]
	level, year := latest(series[models.ColAverageSeries])
	if level <= 0 {
		// The ICP's own total when the yearly one is missing.
		if v := series[models.ColICPTotal][ICPYear]; v > 0 {
			level, year = v, ICPYear
		}
	}
	if level <= 0 {
		return p, 0, ErrNoData
	}
	p.Level = level * d.roll(iso3, year, now)

	p.Rel = map[models.ColGroup]float64{}
	if total := series[models.ColICPTotal][ICPYear]; total > 0 {
		for _, g := range models.ColGroups {
			if v := series[g.ICP][ICPYear]; v > 0 {
				p.Rel[g.Group] = v / total
			}
		}
	}
	return p, year, nil
}

// roll is how a PPP of year moved by now: the country's prices' change over
// the US', year by year (a year either lacks counting as no change).
func (d *Data) roll(iso3 string, year, now int) float64 {
	f := 1.0
	for y := year + 1; y <= now; y++ {
		c, okC := d.inflation[iso3][y]
		us, okUS := d.inflation[usISO3][y]
		if okC && okUS {
			f *= (1 + c/100) / (1 + us/100)
		}
	}
	return f
}

// Inflation is a country's consumer prices' change (%) per year, nil
// without any. Read only.
func (d *Data) Inflation(iso3 string) map[int]float64 {
	if d == nil {
		return nil
	}
	return d.inflation[iso3]
}

// InflationYears are the first and last years of a country's inflation, 0
// without any.
func (d *Data) InflationYears(iso3 string) (first, last int) {
	for y := range d.Inflation(iso3) {
		if first == 0 || y < first {
			first = y
		}
		if y > last {
			last = y
		}
	}
	return first, last
}

// needsBackfill tells the stored inflation starts after inflationSince (kept
// when it was later), so the job fetches the older years right away.
func (d *Data) needsBackfill() bool {
	first, _ := d.InflationYears(usISO3)
	return first > inflationSince
}

// Has tells the country has price data.
func (d *Data) Has(iso3 string) bool {
	_, _, err := d.Prices(iso3, time.Now().Year())
	return err == nil
}

func latest(years map[int]float64) (v float64, year int) {
	for y, x := range years {
		if x > 0 && y > year {
			v, year = x, y
		}
	}
	return v, year
}

var current atomic.Pointer[Data]

// Current is the loaded data, nil before any.
func Current() *Data {
	return current.Load()
}

// SetData replaces the data in memory, giving back a func restoring the
// previous ones. For tests: ppp is country → series → year → PPP.
func SetData(ppp map[string]map[string]map[int]float64, inflation map[string]map[int]float64) (restore func()) {
	prev := current.Load()
	current.Store(&Data{ppp: ppp, inflation: inflation})
	return func() { current.Store(prev) }
}

// Status is the job's in-memory state, since the process started.
type Status struct {
	Running       bool
	LastRunAt     time.Time
	LastSuccessAt time.Time
	NextRunAt     time.Time
	LastDuration  time.Duration
	LastErr       string
	// Countries are the ones with data, Inflation the ones with its rates.
	Countries int
	Inflation int
}

const metaFetchedAt = "col_fetched_at"

type Job struct {
	db       *sql.DB
	q        *sharedqueries.Queries
	interval time.Duration
	client   *http.Client
	urls     sourceURLs
	trigger  chan struct{}

	mu     sync.Mutex
	status Status
}

// NewJob makes a job fetching the price data every interval.
func NewJob(db *sql.DB, q *sharedqueries.Queries, interval time.Duration) *Job {
	return &Job{
		db:       db,
		q:        q,
		interval: interval,
		client:   &http.Client{Timeout: fetchTimeout},
		urls:     defaultURLs,
		trigger:  make(chan struct{}, 1),
	}
}

func (j *Job) Interval() time.Duration { return j.interval }

func (j *Job) Status() Status {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status
}

// FetchedAt is when the stored data were fetched, zero when never.
func (j *Job) FetchedAt(ctx context.Context) time.Time {
	v, err := j.q.GetMeta(ctx, metaFetchedAt)
	if err != nil {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, v)
	return t
}

// Load reads the stored data into memory.
func (j *Job) Load(ctx context.Context) error {
	ppp, err := j.q.ListColPPP(ctx)
	if err != nil {
		return err
	}
	infl, err := j.q.ListColInflation(ctx)
	if err != nil {
		return err
	}

	d := &Data{ppp: map[string]map[string]map[int]float64{}, inflation: map[string]map[int]float64{}}
	for _, r := range ppp {
		if d.ppp[r.Country] == nil {
			d.ppp[r.Country] = map[string]map[int]float64{}
		}
		if d.ppp[r.Country][r.Series] == nil {
			d.ppp[r.Country][r.Series] = map[int]float64{}
		}
		d.ppp[r.Country][r.Series][int(r.Year)] = r.Value
	}
	for _, r := range infl {
		if d.inflation[r.Country] == nil {
			d.inflation[r.Country] = map[int]float64{}
		}
		d.inflation[r.Country][int(r.Year)] = r.Pct
	}
	current.Store(d)

	n := 0
	now := time.Now().Year()
	for _, c := range countries.All() {
		if _, _, err := d.Prices(c.ISO3, now); err == nil {
			n++
		}
	}
	j.mu.Lock()
	j.status.Countries = n
	j.status.Inflation = len(d.inflation)
	j.mu.Unlock()
	return nil
}

// Trigger queues a run, unless one is already running or queued.
func (j *Job) Trigger() bool {
	if j.Status().Running {
		return false
	}
	select {
	case j.trigger <- struct{}{}:
		return true
	default:
		return false
	}
}

// Run fetches the data every interval (right away when the stored ones are
// older than that, missing, or lack the older years of inflation) and on Trigger, until ctx is done.
func (j *Job) Run(ctx context.Context) {
	l := logger.Get()

	wait := time.Duration(0)
	if at := j.FetchedAt(ctx); !at.IsZero() && !Current().needsBackfill() {
		wait = max(j.interval-time.Since(at), 0)
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()
	j.setNextRun(time.Now().Add(wait))

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-j.trigger:
		}

		if err := j.RunOnce(ctx); err != nil {
			l.Error("failed to fetch the cost of living data", zap.Error(err))
		}

		timer.Reset(j.interval)
		j.setNextRun(time.Now().Add(j.interval))
	}
}

func (j *Job) setNextRun(t time.Time) {
	j.mu.Lock()
	j.status.NextRunAt = t
	j.mu.Unlock()
}

// RunOnce fetches the data, stores them and reloads them. A source failing
// keeps its stored data; it fails when every source does.
func (j *Job) RunOnce(ctx context.Context) (err error) {
	l := logger.Get()
	start := time.Now()

	j.mu.Lock()
	if j.status.Running {
		j.mu.Unlock()
		return errors.New("cost of living job already running")
	}
	j.status.Running = true
	j.status.LastRunAt = start
	j.mu.Unlock()

	defer func() {
		j.mu.Lock()
		defer j.mu.Unlock()
		j.status.Running = false
		j.status.LastDuration = time.Since(start)
		j.status.LastErr = ""
		if err != nil {
			j.status.LastErr = err.Error()
		} else {
			j.status.LastSuccessAt = time.Now()
		}
	}()

	f := j.fetch(ctx)
	if len(f.ppp) == 0 && len(f.inflation) == 0 {
		return fmt.Errorf("couldn't fetch any source: %w", errors.Join(f.errs...))
	}
	if err := j.persist(ctx, f, time.Now().UTC()); err != nil {
		return fmt.Errorf("failed to store the cost of living data: %w", err)
	}
	if err := j.Load(ctx); err != nil {
		return fmt.Errorf("failed to load the cost of living data: %w", err)
	}

	l.Info("fetched the cost of living data",
		zap.Duration("took", time.Since(start)),
		zap.Int("ppps", len(f.ppp)),
		zap.Int("inflation_rates", len(f.inflation)),
		zap.Int("failed_sources", len(f.errs)))
	if len(f.errs) > 0 {
		return fmt.Errorf("some sources failed: %w", errors.Join(f.errs...))
	}
	return nil
}

func (j *Job) persist(ctx context.Context, f fetched, at time.Time) error {
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	q := j.q.WithTx(tx)
	for _, p := range f.ppp {
		if err := q.UpsertColPPP(ctx, sharedqueries.UpsertColPPPParams{
			Country: p.country, Series: p.series, Year: int64(p.year), Value: p.value,
		}); err != nil {
			return err
		}
	}
	for _, i := range f.inflation {
		if err := q.UpsertColInflation(ctx, sharedqueries.UpsertColInflationParams{
			Country: i.country, Year: int64(i.year), Pct: i.pct,
		}); err != nil {
			return err
		}
	}
	if err := q.SetMeta(ctx, sharedqueries.SetMetaParams{Key: metaFetchedAt, Value: at.Format(time.RFC3339)}); err != nil {
		return err
	}
	return tx.Commit()
}
