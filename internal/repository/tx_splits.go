package repository

import (
	"context"
	"database/sql"
	"slices"
	"time"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

// itemSharesOf is who share each line of the transaction, by line id.
func itemSharesOf(ctx context.Context, q *queries.Queries, txID string) (map[string][]models.ItemShare, error) {
	rows, err := q.GetTxItemShares(ctx, sql.NullString{String: txID, Valid: true})
	if err != nil {
		logger.FromCtx(ctx).Error("querying item shares", zap.Error(err), zap.String("transaction_id", txID))
		return nil, err
	}
	shares := map[string][]models.ItemShare{}
	for _, row := range rows {
		id := models.SplitMe
		if row.CounterpartyID.Valid {
			id = row.CounterpartyID.String
		}
		shares[row.ItemID] = append(shares[row.ItemID], models.ItemShare{Person: id, Units: row.Units})
	}
	return shares, nil
}

// keepItemSharesWithTxn gives the lines about to replace the transaction's
// the shares the ones with their ids had, unless they come with their own.
func keepItemSharesWithTxn(ctx context.Context, qtx *queries.Queries, txID string, items []models.TxItem) error {
	old, err := qtx.GetTransactionItemsOf(ctx, sql.NullString{String: txID, Valid: true})
	if err != nil {
		logger.FromCtx(ctx).Error("querying items", zap.Error(err), zap.String("transaction_id", txID))
		return err
	}
	if len(old) == 0 {
		return nil
	}
	shares, err := itemSharesOf(ctx, qtx, txID)
	if err != nil {
		return err
	}
	all := make(map[string]bool, len(old))
	for _, it := range old {
		all[it.ID] = it.SharedByAll
	}
	for i := range items {
		it := &items[i]
		if it.ID == "" || it.SharedByAll || it.SharedWith != nil {
			continue
		}
		it.SharedByAll = all[it.ID]
		it.SharedWith = shares[it.ID]
	}
	return nil
}

// createItemSharesWithTxn writes who share the line.
func createItemSharesWithTxn(ctx context.Context, qtx *queries.Queries, itemID string, with []models.ItemShare) error {
	for _, sh := range with {
		if err := qtx.CreateTxItemShare(ctx, queries.CreateTxItemShareParams{
			ItemID:         itemID,
			CounterpartyID: sql.NullString{String: sh.Person, Valid: sh.Person != models.SplitMe},
			Units:          sh.Units,
		}); err != nil {
			logger.FromCtx(ctx).Error("creating item share", zap.Error(err), zap.String("item_id", itemID))
			return err
		}
	}
	return nil
}

// GetTxSplit is the transaction's bill split: its lines with their shares
// and the people it's split with.
func (r *Repository) GetTxSplit(ctx context.Context, txID string) (*models.TxSplit, error) {
	tx, err := r.GetTransaction(ctx, txID)
	if err != nil {
		return nil, err
	}
	return r.TxSplitOf(ctx, tx)
}

// TxSplitOf is the split of tx (loaded with its lines): their shares are set
// on them, People empty when it isn't split.
func (r *Repository) TxSplitOf(ctx context.Context, tx *models.Transaction) (*models.TxSplit, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	shares, err := itemSharesOf(ctx, q, tx.ID)
	if err != nil {
		return nil, err
	}
	for i := range tx.Items {
		tx.Items[i].SharedWith = shares[tx.Items[i].ID]
	}

	rows, err := q.GetTxSplitPeople(ctx, tx.ID)
	if err != nil {
		logger.FromCtx(ctx).Error("querying split people", zap.Error(err), zap.String("transaction_id", tx.ID))
		return nil, err
	}
	s := &models.TxSplit{Tx: tx, People: make([]models.SplitPerson, len(rows))}
	for i, row := range rows {
		s.People[i] = models.SplitPerson{ID: row.ID, Name: row.Name, Icon: row.Icon.String, Color: row.Color.String}
	}
	return s, nil
}

// SaveTxSplit writes who the transaction's bill is split with (counterparty
// ids, in order) and how each of its lines is shared (by line id; lines left
// out are the user's).
func (r *Repository) SaveTxSplit(ctx context.Context, txID string, people []string, shares map[string]models.SplitItemShare) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		l := logger.FromCtx(ctx)

		items, err := qtx.GetTransactionItemsOf(ctx, sql.NullString{String: txID, Valid: true})
		if err != nil {
			l.Error("querying items", zap.Error(err), zap.String("transaction_id", txID))
			return err
		}

		if err := qtx.DeleteTxSplitPeople(ctx, txID); err != nil {
			l.Error("deleting split people", zap.Error(err), zap.String("transaction_id", txID))
			return err
		}
		for i, id := range people {
			if err := qtx.AddTxSplitPerson(ctx, queries.AddTxSplitPersonParams{
				TransactionID: txID, CounterpartyID: id, Position: int64(i),
			}); err != nil {
				l.Error("adding split person", zap.Error(err), zap.String("transaction_id", txID))
				return err
			}
		}

		tx := sql.NullString{String: txID, Valid: true}
		if err := qtx.DeleteTxItemShares(ctx, tx); err != nil {
			l.Error("deleting item shares", zap.Error(err), zap.String("transaction_id", txID))
			return err
		}
		if err := qtx.ClearTxItemsSharedByAll(ctx, tx); err != nil {
			l.Error("clearing items shared by all", zap.Error(err), zap.String("transaction_id", txID))
			return err
		}
		for _, it := range items {
			sh, ok := shares[it.ID]
			if !ok || models.TxItemKindOf(it.Kind).IsCharge() {
				continue
			}
			if sh.All {
				if err := qtx.SetTxItemSharedByAll(ctx, queries.SetTxItemSharedByAllParams{
					SharedByAll: true, ID: it.ID, TransactionID: tx,
				}); err != nil {
					l.Error("sharing item by all", zap.Error(err), zap.String("item_id", it.ID))
					return err
				}
				continue
			}
			with := slices.DeleteFunc(slices.Clone(sh.With), func(sh models.ItemShare) bool {
				return sh.Person != models.SplitMe && !slices.Contains(people, sh.Person)
			})
			if err := createItemSharesWithTxn(ctx, qtx, it.ID, with); err != nil {
				return err
			}
		}
		l.Debug("saved split", zap.String("transaction_id", txID), zap.Int("people", len(people)))
		return nil
	})
}

// RequestSplitPayments schedules an income per share (from its person, on
// day, into the expense's account) for what's left to request of it, each
// reimbursing the expense: what they owe shows as upcoming until they pay,
// fulfilling it then. It's the number of incomes created.
func (r *Repository) RequestSplitPayments(ctx context.Context, expense *models.Transaction, shares []models.SplitShare, day time.Time) (int, error) {
	n := 0
	err := r.transact(ctx, func(qtx *queries.Queries) error {
		l := logger.FromCtx(ctx)

		reimbursed, err := qtx.GetReimbursedTotal(ctx, queries.GetReimbursedTotalParams{
			ID: sql.NullString{String: expense.ID, Valid: true},
		})
		if err != nil {
			l.Error("querying reimbursed total", zap.Error(err), zap.String("id", expense.ID))
			return err
		}
		cur := &expense.Account.Currency
		for _, s := range shares {
			amount := cur.Round(s.Left())
			if amount <= 0 || s.Person.IsMe() {
				continue
			}
			if reimbursed+amount > expense.Amount+amountEpsilon {
				return models.ErrReimbursementExceeds
			}
			reimbursed += amount

			income := &models.Transaction{
				ID:           idgen.New(),
				Account:      expense.Account,
				Amount:       amount,
				Kind:         models.IncomeTxKind,
				Counterparty: &models.Counterparty{ID: s.Person.ID},
				Description:  splitIncomeDescription(expense),
				ScheduledTo:  day,
			}
			if err := r.createTransactionWithTxn(qtx, ctx, income); err != nil {
				return err
			}
			if err := qtx.SetTransactionReimburses(ctx, queries.SetTransactionReimbursesParams{
				ID:           income.ID,
				ReimbursesID: sql.NullString{String: expense.ID, Valid: true},
			}); err != nil {
				l.Error("linking split payment", zap.Error(err), zap.String("id", income.ID))
				return err
			}
			n++
		}
		l.Info("requested split payments", zap.String("transaction_id", expense.ID), zap.Int("count", n))
		return nil
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}

// splitIncomeDescription names a share's income after its bill.
func splitIncomeDescription(expense *models.Transaction) string {
	switch {
	case expense.Description != "":
		return "Share of " + expense.Description
	case expense.Counterparty != nil:
		return "Share of " + expense.Counterparty.Name
	default:
		return "Share of the bill"
	}
}
