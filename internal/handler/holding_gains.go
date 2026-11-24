package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/repository"
	"github.com/omareloui/tyche/web/views"
	"go.uber.org/zap"
)

// holdingDailyDays is how far back the holdings' gains take each day's
// rate; older days take their week's first (fewer to fetch on a first
// view, as fx_rates_daily keeps them after).
const holdingDailyDays = 365

// holdingGains are the gains of every account in another currency than
// the default one (models.ComputeHoldingGains), by account id, with the
// accounts.
func (h *Handler) holdingGains(ctx context.Context) (map[string]*models.HoldingGain, []repository.HoldingAccountInfo, error) {
	def := &getClaims(ctx).DefaultCurrency
	accs, moves, err := h.repo.GetHoldingData(ctx, def)
	if err != nil || len(accs) == 0 {
		return nil, nil, err
	}
	hold := make([]models.HoldingAccount, len(accs))
	for i, a := range accs {
		hold[i] = a.HoldingAccount
	}

	now := time.Now().UTC()
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -holdingDailyDays)
	snap := func(d time.Time) time.Time {
		d = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
		if d.Before(cutoff) {
			d = d.AddDate(0, 0, -(int(d.Weekday())+6)%7)
		}
		return d
	}

	var rates map[time.Time]map[string]float64
	if h.fxJob != nil {
		days, codes := models.HoldingRateNeeds(def.String, hold, moves)
		seen := make(map[time.Time]bool, len(days))
		snapped := days[:0]
		for _, d := range days {
			if d = snap(d); !seen[d] {
				seen[d] = true
				snapped = append(snapped, d)
			}
		}
		if len(snapped) > 0 {
			rates, _ = h.fxJob.RatesOn(ctx, snapped, codes, def.String)
		}
	}
	l := logger.FromCtx(ctx)
	today := func(code string) float64 {
		v, err := fxrate.GetFxRate(code, def.String)
		if err != nil {
			l.Warn("no rate for a holding's gain", zap.String("currency", code), zap.Error(err))
			return 0
		}
		return v
	}
	rateOn := func(day time.Time, code string) (float64, bool) {
		if v, ok := rates[snap(day)][strings.ToLower(code)]; ok && v > 0 {
			return v, true
		}
		return today(code), false
	}
	return models.ComputeHoldingGains(def.String, hold, moves, rateOn, today), accs, nil
}

// AccountsSummaryGains is the gains of the accounts summary's currencies
// (the accounts it counts, with its `with`), swapped into their rows.
func (h *Handler) AccountsSummaryGains(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	gains, accs, err := h.holdingGains(ctx)
	if err != nil {
		return nil, err
	}
	with := accountsSummaryWith(r)
	byCode := map[string][]*models.HoldingGain{}
	for _, a := range accs {
		g := gains[a.ID]
		switch {
		case !a.Shown || g.Units <= 0:
			continue
		case a.GoalID != "":
			if !with.Goals {
				continue
			}
		case a.Excluded && !with.Excluded:
			continue
		}
		byCode[g.Code] = append(byCode[g.Code], g)
	}
	curs, err := h.repo.GetCurrencies(ctx)
	if err != nil {
		return nil, err
	}
	var rows []views.HoldingGainRow
	for i := range curs {
		c := &curs[i]
		if gs := byCode[strings.ToUpper(c.String)]; len(gs) > 0 {
			rows = append(rows, views.HoldingGainRow{Currency: c, Gain: models.SumHoldingGains(gs...)})
		}
	}
	return views.AccSummaryGains(&getClaims(ctx).DefaultCurrency, rows), nil
}

// AccountGain is an account's cost, value and gain, for its details;
// nothing for one in the default currency or not holding money.
func (h *Handler) AccountGain(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	a, err := h.repo.GetAccount(ctx, r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	def := &getClaims(ctx).DefaultCurrency
	if strings.EqualFold(a.Currency.String, def.String) {
		return templ.NopComponent, nil
	}
	gains, accs, err := h.holdingGains(ctx)
	if err != nil {
		return nil, err
	}
	for _, info := range accs {
		if info.ID == a.ID && info.Shown {
			return views.HoldingGainDetails(def, &a.Currency, gains[a.ID]), nil
		}
	}
	return templ.NopComponent, nil
}

// GoalGains are the cost, value and gain of a goal's accounts in another
// currency than the default one, for its details.
func (h *Handler) GoalGains(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	ctx := r.Context()
	id := r.PathValue("id")
	if id == "" {
		return nil, errs.ErrInvalidID
	}
	gl, err := h.repo.GetGoal(ctx, id)
	if err != nil {
		return nil, err
	}
	def := &getClaims(ctx).DefaultCurrency
	funded := gl.Funded()
	foreign := false
	for _, a := range funded {
		foreign = foreign || !strings.EqualFold(a.Currency.String, def.String)
	}
	if !gl.FulfilledAt.IsZero() || !foreign {
		return templ.NopComponent, nil
	}
	gains, _, err := h.holdingGains(ctx)
	if err != nil {
		return nil, err
	}
	var rows []views.HoldingGainRow
	for i := range funded {
		a := &funded[i]
		if g := gains[a.ID]; g.Shows() {
			rows = append(rows, views.HoldingGainRow{Currency: &a.Currency, Gain: g})
		}
	}
	return views.GoalHoldingGains(def, rows), nil
}
