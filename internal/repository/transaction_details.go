package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

// txDetailsRelated is how many related transactions (other occurrences, with
// the counterparty) the details show.
const txDetailsRelated = 5

// GetTransactionDetails is the transaction with what's related to it: its
// SMS, recurrent transaction, the expense it reimburses, its transfer pair's
// other side, the latest transactions with its counterparty and the stats of
// its month (counterparty totals and category budget, in def). The SMS
// template name is left to the caller, as the shared ones aren't in the
// tenant DB.
func (r *Repository) GetTransactionDetails(ctx context.Context, id string, def *models.Currency) (*models.TxDetails, error) {
	l := logger.FromCtx(ctx)

	tx, err := r.GetTransaction(ctx, id)
	if err != nil {
		return nil, err
	}

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	d := &models.TxDetails{Tx: tx, Month: models.MonthOf(tx.ScheduledTo)}

	if tx.FromSMS {
		row, err := q.GetSMSMessageOfTransaction(ctx, sql.NullString{String: id, Valid: true})
		switch {
		case err == nil:
			d.SMS = dbSMSMessageToSMSMessage(&row.SmsMessage)
		case !errors.Is(err, sql.ErrNoRows):
			l.Error("querying transaction sms", zap.Error(err), zap.String("id", id))
			return nil, err
		}
	}

	if tx.ReimbursesID != "" {
		if d.Reimburses, err = r.GetTransaction(ctx, tx.ReimbursesID); err != nil {
			return nil, err
		}
	}

	if tx.IsTransferLeg() {
		if tx.TransferPair != nil {
			d.Pair = tx.TransferPair
		} else if d.Pair, err = r.GetTransaction(ctx, tx.TransferPairID); err != nil {
			return nil, err
		}
	}

	if tx.RecurrentTxID != "" {
		rt, err := r.GetRecurrentTx(ctx, tx.RecurrentTxID)
		switch {
		case err == nil:
			d.Recurrent = rt
		case !errors.Is(err, sql.ErrNoRows):
			return nil, err
		}
		if d.RecurrentTxs, err = r.relatedTransactions(ctx, tx.ID, queries.GetTransactionsParams{RecurrentTxID: tx.RecurrentTxID}); err != nil {
			return nil, err
		}
	}

	if tx.Counterparty != nil {
		if d.CounterpartyTxs, err = r.relatedTransactions(ctx, tx.ID, queries.GetTransactionsParams{CounterpartyID: tx.Counterparty.ID}); err != nil {
			return nil, err
		}
		if d.CounterpartyMonth, err = r.GetTransactionsSummary(ctx, models.TxFilter{
			CounterpartyID: tx.Counterparty.ID,
			From:           d.Month,
			To:             d.Month.AddDate(0, 1, -1),
		}, def); err != nil {
			return nil, err
		}
	}

	if tx.Category != nil && tx.Kind.CategoryKind() == models.ExpenseTxKind {
		bs, err := r.GetBudgetSummary(ctx, d.Month, def, nil)
		if err != nil {
			return nil, err
		}
		for i := range bs.Items {
			if c := bs.Items[i].Category; c != nil && c.ID == tx.Category.ID {
				d.Budget = &bs.Items[i]
				break
			}
		}
	}

	return d, nil
}

// relatedTransactions are the latest transactions matching p but the one of
// id, at most txDetailsRelated of them.
func (r *Repository) relatedTransactions(ctx context.Context, id string, p queries.GetTransactionsParams) ([]models.Transaction, error) {
	p.Lim = txDetailsRelated + 1
	// Not the scheduled ones far in the future, e.g. a recurrent's.
	p.Before = time.Now().AddDate(0, 1, 0)
	txs, err := r.listTransactions(ctx, p)
	if err != nil {
		return nil, err
	}
	related := make([]models.Transaction, 0, txDetailsRelated)
	for _, t := range txs {
		if t.ID != id && len(related) < txDetailsRelated {
			related = append(related, t)
		}
	}
	return related, nil
}
