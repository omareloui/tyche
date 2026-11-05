package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

// CreateGamyaa creates the gam'yaa account a (with a.Gamyaa set) along with
// its schedule: a monthly recurrent transfer from its pay-from account
// generating all its installments, and a scheduled payout transfer back per
// turn.
func (r *Repository) CreateGamyaa(ctx context.Context, a *models.Account) error {
	l := logger.FromCtx(ctx)
	g := a.Gamyaa
	now := time.Now()

	turns, err := json.Marshal(g.Turns)
	if err != nil {
		return err
	}

	return r.transact(ctx, func(qtx *queries.Queries) error {
		if err := r.createAccountWithTxn(qtx, ctx, a); err != nil {
			return err
		}

		rt := &models.RecurrentTx{
			ID:          idgen.New(),
			Unit:        models.RecurrentUnitMonth,
			Every:       1,
			Account:     *g.PayFrom,
			Amount:      g.MonthlyPayment(),
			Kind:        models.TransferTxKind,
			ToAccount:   a,
			ToAmount:    g.MonthlyPayment(),
			Description: a.Name + " installment",
			StartsAt:    g.StartsOn,
			StopsAt:     g.EndsOn(),
			// Its installments are all written below, none left to write.
			Anchor:    g.EndsOn(),
			NextIndex: 1,
			CreatedAt: now,
		}
		if err := r.createRecurrentTxWithTxn(qtx, ctx, rt); err != nil {
			return err
		}
		g.RecurrentTxID = rt.ID

		txs := append(g.Installments(a, &models.RecurrentTx{ID: rt.ID}, now), g.Payouts(a, now)...)
		for i := range txs {
			txs[i].ID = idgen.New()
			if err := r.createTransactionWithTxn(qtx, ctx, &txs[i]); err != nil {
				return err
			}
		}

		if err := qtx.CreateGamyaa(ctx, queries.CreateGamyaaParams{
			AccountID:        a.ID,
			Installment:      g.Installment,
			Members:          int64(g.Members),
			Shares:           g.Shares,
			Turns:            turns,
			StartsOn:         g.StartsOn,
			PayFromAccountID: sql.NullString{String: g.PayFrom.ID, Valid: true},
			RecurrentTxID:    sql.NullString{String: rt.ID, Valid: true},
		}); err != nil {
			l.Error("creating gamyaa", zap.Error(err))
			return err
		}
		l.Debug("created gamyaa", zap.Any("gamyaa", g))
		return nil
	})
}

// attachGamyaas sets Gamyaa, with its progress, on the gam'yaa accounts of
// accs.
func (r *Repository) attachGamyaas(ctx context.Context, q *queries.Queries, accs []models.Account) error {
	l := logger.FromCtx(ctx)

	idx := map[string]int{}
	for i, a := range accs {
		if a.Kind.Enum == models.AccKindGamyaa {
			idx[a.ID] = i
		}
	}
	if len(idx) == 0 {
		return nil
	}

	dbGs, err := q.GetGamyaas(ctx)
	if err != nil {
		l.Error("querying gamyaas", zap.Error(err))
		return err
	}
	for _, row := range dbGs {
		i, ok := idx[row.Gamyaa.AccountID]
		if !ok {
			continue
		}
		accs[i].Gamyaa = dbGamyaaToGamyaa(&row)
	}

	dbTxs, err := q.GetGamyaaTransactions(ctx)
	if err != nil {
		l.Error("querying gamyaa transactions", zap.Error(err))
		return err
	}
	for _, t := range dbTxs {
		if i, ok := idx[t.ToAccountID.String]; ok && t.ToAccountID.Valid && accs[i].Gamyaa != nil {
			g := accs[i].Gamyaa
			if t.FulfilledAt.Valid {
				g.PaidCount++
				g.PaidAmount += t.ToAmount.Float64
			} else {
				g.ToPay += t.ToAmount.Float64
				if g.NextPaymentOn.IsZero() {
					g.NextPaymentOn = t.ScheduledTo
				}
			}
			g.AddMonthTx(t.ScheduledTo, t.ToAmount.Float64, false, t.FulfilledAt.Valid)
		}
		if i, ok := idx[t.AccountID]; ok && accs[i].Gamyaa != nil {
			g := accs[i].Gamyaa
			if t.FulfilledAt.Valid {
				g.ReceivedCount++
				g.ReceivedAmount += t.Amount
			} else {
				g.ToReceive += t.Amount
				if g.NextPayoutOn.IsZero() {
					g.NextPayoutOn = t.ScheduledTo
				}
			}
			g.AddMonthTx(t.ScheduledTo, t.Amount, true, t.FulfilledAt.Valid)
		}
	}
	return nil
}

func dbGamyaaToGamyaa(row *queries.GetGamyaasRow) *models.Gamyaa {
	g := &models.Gamyaa{
		Installment:   row.Gamyaa.Installment,
		Members:       int(row.Gamyaa.Members),
		Shares:        row.Gamyaa.Shares,
		StartsOn:      row.Gamyaa.StartsOn,
		RecurrentTxID: row.Gamyaa.RecurrentTxID.String,
	}
	_ = json.Unmarshal(row.Gamyaa.Turns, &g.Turns)
	if row.Gamyaa.PayFromAccountID.Valid {
		g.PayFrom = &models.Account{ID: row.Gamyaa.PayFromAccountID.String, Name: row.PayFromAccountName.String}
	}
	return g
}
