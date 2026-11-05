// Package fxrate keeps the exchange rates in memory for every user. A Job
// fetches them (fawazahmed0 currency-api) periodically into the shared
// database's fx_rates table and swaps the in-memory rates when done.
package fxrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mattn/go-sqlite3"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/sharedqueries"
	"go.uber.org/zap"
)

var ErrNoRate = errors.New("no fx rate")

// rates are base → quote → rate, with lowercase codes. A loaded map is never
// mutated, it's replaced as a whole.
type rates map[string]map[string]float64

var current atomic.Pointer[rates]

// crossBase is used to derive a pair that wasn't fetched directly.
const crossBase = "usd"

// GetFxRate returns the rate to convert from a currency to another.
func GetFxRate(from, to string) (float64, error) {
	from = strings.ToLower(from)
	to = strings.ToLower(to)

	if from == to {
		return 1, nil
	}

	r := current.Load()
	if r == nil {
		return 0, ErrNoRate
	}

	if v := (*r)[from][to]; v > 0 {
		return v, nil
	}

	if fromV, toV := (*r)[crossBase][from], (*r)[crossBase][to]; fromV > 0 && toV > 0 {
		return toV / fromV, nil
	}

	return 0, ErrNoRate
}

func setRates(r rates) {
	current.Store(&r)
}

// SetRates replaces the rates in memory (base → quote → rate, lowercase
// codes), giving back a func restoring the previous ones. For tests.
func SetRates(r map[string]map[string]float64) (restore func()) {
	prev := current.Load()
	setRates(r)
	return func() { current.Store(prev) }
}

var defaultURLs = []string{
	"https://cdn.jsdelivr.net/npm/@fawazahmed0/currency-api@latest/v1/currencies/%s.min.json",
	"https://latest.currency-api.pages.dev/v1/currencies/%s.min.json",
}

const (
	fetchWorkers = 8
	fetchTimeout = 30 * time.Second
)

// Status is the job's in-memory state, since the process started.
type Status struct {
	Running       bool
	LastRunAt     time.Time
	LastSuccessAt time.Time
	NextRunAt     time.Time
	LastDuration  time.Duration
	LastErr       string
	FailedBases   []string
}

// Info describes the rates stored in the shared database.
type Info struct {
	Pairs     int64
	Bases     int64
	AsOf      string
	FetchedAt time.Time
}

type Job struct {
	db       *sql.DB
	q        *sharedqueries.Queries
	codes    []string
	interval time.Duration
	client   *http.Client
	urls     []string
	trigger  chan struct{}

	mu     sync.Mutex
	status Status
}

// NewJob makes a job fetching the rates between the given currency codes.
func NewJob(db *sql.DB, q *sharedqueries.Queries, codes []string, interval time.Duration) *Job {
	lower := make([]string, len(codes))
	for i, c := range codes {
		lower[i] = strings.ToLower(c)
	}
	slices.Sort(lower)

	return &Job{
		db:       db,
		q:        q,
		codes:    slices.Compact(lower),
		interval: interval,
		client:   &http.Client{Timeout: fetchTimeout},
		urls:     defaultURLs,
		trigger:  make(chan struct{}, 1),
	}
}

func (j *Job) Interval() time.Duration {
	return j.interval
}

func (j *Job) Status() Status {
	j.mu.Lock()
	defer j.mu.Unlock()
	s := j.status
	s.FailedBases = slices.Clone(s.FailedBases)
	return s
}

func (j *Job) Info(ctx context.Context) (Info, error) {
	row, err := j.q.GetFxRatesInfo(ctx)
	if err != nil {
		return Info{}, err
	}
	info := Info{
		Pairs:     row.Pairs,
		Bases:     row.Bases,
		FetchedAt: parseSQLiteTime(row.FetchedAt),
	}
	if asOf := parseSQLiteTime(row.AsOf); !asOf.IsZero() {
		info.AsOf = asOf.Format(time.DateOnly)
	}
	return info, nil
}

// Load reads the stored rates into memory.
func (j *Job) Load(ctx context.Context) error {
	rows, err := j.q.ListFxRates(ctx)
	if err != nil {
		return err
	}

	r := make(rates)
	for _, row := range rows {
		if r[row.Base] == nil {
			r[row.Base] = make(map[string]float64, len(j.codes))
		}
		r[row.Base][row.Quote] = row.Rate
	}
	setRates(r)
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

// Run fetches the rates every interval (right away when the stored ones are
// older than that) and on Trigger, until ctx is done.
func (j *Job) Run(ctx context.Context) {
	l := logger.Get()

	wait := time.Duration(0)
	if info, err := j.Info(ctx); err != nil {
		l.Error("failed to get the fx rates info", zap.Error(err))
	} else if !info.FetchedAt.IsZero() {
		wait = max(j.interval-time.Since(info.FetchedAt), 0)
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
			l.Error("failed to fetch fx rates", zap.Error(err))
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

// RunOnce fetches all the rates, stores them, and swaps the in-memory ones.
// Bases that fail to fetch keep their previously stored rates.
func (j *Job) RunOnce(ctx context.Context) (err error) {
	l := logger.Get()
	start := time.Now()

	j.mu.Lock()
	if j.status.Running {
		j.mu.Unlock()
		return errors.New("fx rates job already running")
	}
	j.status.Running = true
	j.status.LastRunAt = start
	j.mu.Unlock()

	var failed []string
	defer func() {
		j.mu.Lock()
		defer j.mu.Unlock()
		j.status.Running = false
		j.status.LastDuration = time.Since(start)
		j.status.FailedBases = failed
		j.status.LastErr = ""
		if err != nil {
			j.status.LastErr = err.Error()
		} else {
			j.status.LastSuccessAt = time.Now()
		}
	}()

	fetched, failed := j.fetchAll(ctx)
	if len(fetched) == 0 {
		return fmt.Errorf("couldn't fetch any of the %d bases", len(j.codes))
	}

	if err := j.persist(ctx, fetched, time.Now().UTC()); err != nil {
		return fmt.Errorf("failed to store the fx rates: %w", err)
	}

	if err := j.Load(ctx); err != nil {
		return fmt.Errorf("failed to load the fx rates: %w", err)
	}

	l.Info("fetched fx rates",
		zap.Duration("took", time.Since(start)),
		zap.Int("bases", len(fetched)),
		zap.Strings("failed", failed))
	return nil
}

type baseRates struct {
	base  string
	asOf  time.Time
	rates map[string]float64
}

func (j *Job) fetchAll(ctx context.Context) ([]baseRates, []string) {
	l := logger.Get()

	wanted := make(map[string]bool, len(j.codes))
	for _, c := range j.codes {
		wanted[c] = true
	}

	jobs := make(chan string)
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		fetched []baseRates
		failed  []string
	)

	for range min(fetchWorkers, len(j.codes)) {
		wg.Go(func() {
			for base := range jobs {
				br, err := j.fetchBase(ctx, base, wanted)
				mu.Lock()
				if err != nil {
					l.Warn("failed to fetch fx rates", zap.String("base", base), zap.Error(err))
					failed = append(failed, base)
				} else {
					fetched = append(fetched, br)
				}
				mu.Unlock()
			}
		})
	}

	for _, c := range j.codes {
		select {
		case jobs <- c:
		case <-ctx.Done():
		}
	}
	close(jobs)
	wg.Wait()

	slices.Sort(failed)
	return fetched, failed
}

// fetchBase tries each url in order, keeping only the wanted quotes.
func (j *Job) fetchBase(ctx context.Context, base string, wanted map[string]bool) (baseRates, error) {
	var errs []error
	for _, u := range j.urls {
		br, err := j.fetchURL(ctx, fmt.Sprintf(u, base), base, wanted)
		if err == nil {
			return br, nil
		}
		errs = append(errs, err)
	}
	return baseRates{}, errors.Join(errs...)
}

func (j *Job) fetchURL(ctx context.Context, url, base string, wanted map[string]bool) (baseRates, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return baseRates{}, err
	}

	resp, err := j.client.Do(req)
	if err != nil {
		return baseRates{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return baseRates{}, fmt.Errorf("%s: unexpected status %d", url, resp.StatusCode)
	}

	var body map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return baseRates{}, fmt.Errorf("%s: %w", url, err)
	}

	var date string
	if err := json.Unmarshal(body["date"], &date); err != nil {
		return baseRates{}, fmt.Errorf("%s: invalid date: %w", url, err)
	}
	asOf, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return baseRates{}, fmt.Errorf("%s: invalid date: %w", url, err)
	}

	var all map[string]float64
	if err := json.Unmarshal(body[base], &all); err != nil {
		return baseRates{}, fmt.Errorf("%s: invalid rates: %w", url, err)
	}

	r := make(map[string]float64, len(wanted))
	for quote, v := range all {
		if wanted[quote] && quote != base && v > 0 {
			r[quote] = v
		}
	}
	if len(r) == 0 {
		return baseRates{}, fmt.Errorf("%s: no rates", url)
	}

	return baseRates{base: base, asOf: asOf, rates: r}, nil
}

func (j *Job) persist(ctx context.Context, fetched []baseRates, fetchedAt time.Time) error {
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	q := j.q.WithTx(tx)
	for _, br := range fetched {
		for quote, v := range br.rates {
			if err := q.UpsertFxRate(ctx, sharedqueries.UpsertFxRateParams{
				Base:      br.base,
				Quote:     quote,
				Rate:      v,
				AsOf:      br.asOf,
				FetchedAt: fetchedAt,
			}); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func parseSQLiteTime(s string) time.Time {
	for _, layout := range sqlite3.SQLiteTimestampFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
