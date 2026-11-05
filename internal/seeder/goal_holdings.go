package seeder

import (
	"context"
	"database/sql"

	"go.uber.org/zap"

	"github.com/omareloui/tyche/internal/dbtypes"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
)

// goalHoldings ties the goals' own accounts to them (accounts.goal_id), for
// the goals made before goals held money per currency. A goal still saved
// for gets its money put in from other currencies moved to holdings first
// (see splitGoal); a bought one keeps it as is, its value settled when
// bought. Each goal is split under a savepoint of the seed's transaction tx:
// failing, it's left as it was, tried again on the next seed.
func (s *Seeder) goalHoldings(ctx context.Context, tx *sql.Tx, q *queries.Queries) {
	l := logger.FromCtx(ctx)

	gls, err := q.GetGoalsToSplit(ctx)
	if err != nil || len(gls) == 0 {
		if err != nil {
			l.Error("getting goals to split", zap.Error(err))
		}
		return
	}
	kindID, err := q.GetAccountKindID(ctx, int64(models.AccKindGoal))
	if err != nil {
		l.Error("getting the goal account kind", zap.Error(err))
		return
	}

	for _, gl := range gls {
		if _, err := tx.ExecContext(ctx, "SAVEPOINT goal_split"); err != nil {
			l.Error("starting goal split savepoint", zap.Error(err))
			return
		}
		err := q.SetAccountGoal(ctx, queries.SetAccountGoalParams{
			ID:     gl.AccountID.String,
			GoalID: sql.NullString{String: gl.ID, Valid: true},
		})
		if err == nil && !gl.FulfilledAt.Valid {
			err = splitGoal(ctx, q, kindID, &gl)
		}
		if err != nil {
			l.Error("splitting goal into holdings", zap.String("goal", gl.ID), zap.Error(err))
			if _, err := tx.ExecContext(ctx, "ROLLBACK TO goal_split"); err != nil {
				l.Error("rolling back goal split", zap.Error(err))
				return
			}
		}
		if _, err := tx.ExecContext(ctx, "RELEASE goal_split"); err != nil {
			l.Error("releasing goal split savepoint", zap.Error(err))
			return
		}
	}
}

// splitGoal moves the goal's transfers with accounts of other currencies to
// holdings, oldest first: every deposit, now the same amount into the
// holding of its currency, and the withdraws the holding of their
// currency covers, now the same amount out of it (the others stay
// conversions out of its own account). The accounts' balances follow.
func splitGoal(ctx context.Context, q *queries.Queries, kindID string, gl *queries.GetGoalsToSplitRow) error {
	main := gl.AccountID.String
	txs, err := q.GetGoalAccountTransfers(ctx, main)
	if err != nil {
		return err
	}

	holdings := map[string]string{} // currency id → account id
	held := map[string]float64{}    // currency id → fulfilled money in its holding
	delta := map[string]float64{}   // account id → balance change
	holding := func(curID, cur string) (string, error) {
		if id, ok := holdings[curID]; ok {
			return id, nil
		}
		id := idgen.New()
		if err := q.CreateAccount(ctx, queries.CreateAccountParams{
			ID: id, Name: models.GoalHoldingName(gl.Name, cur), KindID: kindID, CurrencyID: curID,
			ExcludeFromBalance: true, SmsRefs: dbtypes.JSON("[]"),
		}); err != nil {
			return "", err
		}
		if err := q.SetAccountGoal(ctx, queries.SetAccountGoalParams{ID: id, GoalID: sql.NullString{String: gl.ID, Valid: true}}); err != nil {
			return "", err
		}
		holdings[curID] = id
		return id, nil
	}

	for _, t := range txs {
		if t.OtherCurrencyID == gl.CurrencyID {
			continue
		}
		toAmount := t.Amount
		if t.ToAmount.Valid {
			toAmount = t.ToAmount.Float64
		}
		fulfilled := t.FulfilledAt.Valid
		if t.ToAccountID.String == main {
			h, err := holding(t.OtherCurrencyID, t.OtherCurrency)
			if err != nil {
				return err
			}
			if err := q.MoveTransferTo(ctx, queries.MoveTransferToParams{
				ID: t.ID, ToAccountID: sql.NullString{String: h, Valid: true}, ToAmount: sql.NullFloat64{Float64: t.Amount, Valid: true},
			}); err != nil {
				return err
			}
			if fulfilled {
				delta[main] -= toAmount
				delta[h] += t.Amount
				held[t.OtherCurrencyID] += t.Amount
			}
			continue
		}
		h, ok := holdings[t.OtherCurrencyID]
		if !ok || !fulfilled || held[t.OtherCurrencyID] < toAmount-0.005 {
			continue
		}
		if err := q.MoveTransferFrom(ctx, queries.MoveTransferFromParams{ID: t.ID, AccountID: h, Amount: toAmount}); err != nil {
			return err
		}
		delta[main] += t.Amount
		delta[h] -= toAmount
		held[t.OtherCurrencyID] -= toAmount
	}

	for id, d := range delta {
		amount, err := q.GetAccountAmount(ctx, id)
		if err != nil {
			return err
		}
		if err := q.SetAccountAmount(ctx, queries.SetAccountAmountParams{ID: id, Amount: amount + d}); err != nil {
			return err
		}
	}
	return nil
}
