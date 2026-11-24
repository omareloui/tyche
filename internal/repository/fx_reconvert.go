package repository

// One-off: converts every transaction paid in another currency than its
// account's again, at its day's rate plus its account's foreign currency fee
// (expenses), as the ones read from SMS before were converted at the rate of
// the moment they were read and without a fee. Run by the daily upkeep until
// it went through for every one, then marked done on the user row
// (fx_reconverted_at). Delete this file (and its call in the handler's
// DailyUpkeep) once every tenant has it set.

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

// RateOnFunc is from → to on day; dated tells it's that day's rate rather
// than today's fallback.
type RateOnFunc func(ctx context.Context, day time.Time, from, to string) (rate float64, dated bool, err error)

// ReconvertForeignTxs converts the foreign transactions again at their day's
// rate (rateOn, which fetches and keeps the days it lacks) plus the fee on
// expenses, moving the fulfilled ones' accounts by the difference. A
// transaction whose day's rate can't be had is left for the next run; once
// none is, the tenant is marked done and later runs do nothing.
func (r *Repository) ReconvertForeignTxs(ctx context.Context, rateOn RateOnFunc) error {
	l := logger.FromCtx(ctx)
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	done, err := q.GetFxReconvertedAt(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if done.Valid || errors.Is(err, sql.ErrNoRows) {
		return nil
	}

	rows, err := q.GetForeignTxsToReconvert(ctx)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	type rateKey struct {
		day      time.Time
		from, to string
	}
	rates := map[rateKey]float64{}
	rateOf := func(day time.Time, from, to string) float64 {
		k := rateKey{day, strings.ToLower(from), strings.ToLower(to)}
		if v, ok := rates[k]; ok {
			return v
		}
		v, dated, err := rateOn(ctx, day, from, to)
		// Today's (or a day ahead's) rate is today's; a past day's must
		// be that day's.
		if err != nil || v <= 0 || (!dated && day.Before(today)) {
			v = 0
		}
		rates[k] = v
		return v
	}

	// The rates first: fetching a missing day mustn't hold the tenant's
	// connection.
	dayOf := func(t time.Time) time.Time {
		// The day it happened, as stored (SMS ones in the user's zone).
		y, m, d := t.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	for _, row := range rows {
		rateOf(dayOf(row.ScheduledTo), row.OriginalCurrency, row.AccountCurrency)
	}

	updated := 0
	complete := true
	err = r.transact(ctx, func(qtx *queries.Queries) error {
		// Read again, as they may have changed while fetching.
		rows, err := qtx.GetForeignTxsToReconvert(ctx)
		if err != nil {
			return err
		}
		for _, row := range rows {
			rate := rates[rateKey{dayOf(row.ScheduledTo), strings.ToLower(row.OriginalCurrency), strings.ToLower(row.AccountCurrency)}]
			if rate == 0 {
				// Not had (or new since): left for the next run.
				complete = false
				continue
			}
			kind := models.TxKind(row.Kind)
			acc := &models.Account{
				Currency:     models.Currency{String: row.AccountCurrency},
				FxFeePercent: row.FxFeePercent,
				FxFeeMin:     row.FxFeeMin,
			}
			amount, fee := models.ForeignCharge(row.OriginalAmount, rate, acc, kind)
			if err := qtx.SetTransactionConversion(ctx, queries.SetTransactionConversionParams{
				ID: row.ID, Amount: amount, FxFee: sql.NullFloat64{Float64: fee, Valid: fee > 0},
			}); err != nil {
				return err
			}
			updated++
			diff := amount - row.Amount
			if !row.Fulfilled || diff == 0 {
				continue
			}
			// An expense takes from its account, an income or a refund adds.
			if kind == models.ExpenseTxKind {
				diff = -diff
			}
			if err := qtx.AddToAccount(ctx, queries.AddToAccountParams{ID: row.AccountID, Amount: diff}); err != nil {
				return err
			}
		}
		if complete {
			return qtx.SetFxReconverted(ctx)
		}
		return nil
	})
	if err != nil {
		return err
	}
	l.Info("reconverted foreign transactions", zap.Int("updated", updated), zap.Bool("complete", complete))
	return nil
}
