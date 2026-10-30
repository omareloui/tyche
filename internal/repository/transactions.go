package repository

import (
	"context"
	"database/sql"
	"slices"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

func (r *Repository) CreateTransaction(ctx context.Context, tx *models.Transaction, rt *models.RecurrentTx, scheduledTxs []models.Transaction) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		if rt != nil {
			if err := r.createRecurrentTxWithTxn(qtx, ctx, rt); err != nil {
				return err
			}
		}

		if err := r.createTransactionAndApplyToAccountIfNeededWithTxn(ctx, qtx, tx); err != nil {
			return err
		}

		for _, stx := range scheduledTxs {
			if err := r.createTransactionWithTxn(qtx, ctx, &stx); err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *Repository) createTransactionWithTxn(qtx *queries.Queries, ctx context.Context, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)

	var catID, cpID, toAccID, goalID, recurrentID sql.NullString

	if tx.Category != nil {
		catID.Valid = tx.Category.ID != ""
		catID.String = tx.Category.ID
	}
	if tx.Counterparty != nil {
		cpID.Valid = tx.Counterparty.ID != ""
		cpID.String = tx.Counterparty.ID
	}
	if tx.ToAccount != nil {
		toAccID.Valid = tx.ToAccount.ID != ""
		toAccID.String = tx.ToAccount.ID
	}
	if tx.Goal != nil {
		goalID.Valid = tx.Goal.ID != ""
		goalID.String = tx.Goal.ID
	}
	if tx.RecurrentTx != nil {
		recurrentID.Valid = tx.RecurrentTx.ID != ""
		recurrentID.String = tx.RecurrentTx.ID
	}

	if err := qtx.CreateTransaction(ctx, queries.CreateTransactionParams{
		ID:             tx.ID,
		AccountID:      tx.Account.ID,
		Amount:         tx.Amount,
		Kind:           int64(tx.Kind),
		CategoryID:     catID,
		CounterpartyID: cpID,
		Description:    sql.NullString{String: tx.Description, Valid: tx.Description != ""},
		ToAccountID:    toAccID,
		ToAmount:       sql.NullFloat64{Float64: tx.ToAmount, Valid: tx.ToAmount != 0},
		GoalID:         goalID,
		RecurrentID:    recurrentID,
		ScheduledTo:    tx.ScheduledTo,
		FulfilledAt:    sql.NullTime{Time: tx.FulfilledAt, Valid: !tx.FulfilledAt.IsZero()},
		NeedsReview:    tx.NeedsReview,
	}); err != nil {
		l.Error("create transaction", zap.Error(err), zap.Any("transaction", tx))
		return err
	}
	l.Debug("created transaction", zap.Any("transaction", tx))
	return nil
}

func (r *Repository) createTransactionAndApplyToAccountIfNeededWithTxn(ctx context.Context, qtx *queries.Queries, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)
	if err := r.createTransactionWithTxn(qtx, ctx, tx); err != nil {
		return err
	}

	if !tx.FulfilledAt.IsZero() {
		if err := r.applyTxToAccountWithTxn(qtx, ctx, tx); err != nil {
			l.Error("applying transaction", zap.Error(err), zap.Any("transaction", tx))
			return err
		}
		l.Debug("applied transaction", zap.Any("transaction", tx))
	}

	return nil
}

func (r *Repository) GetTransactions(ctx context.Context) ([]models.Transaction, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbTransactions, err := q.GetTransactions(ctx)
	if err != nil {
		l.Error("querying transactions", zap.Error(err))
		return nil, err
	}
	l.Info("queried transactions")

	tx := make([]models.Transaction, len(dbTransactions))
	for i, t := range dbTransactions {
		tx[i] = *dbTransactionRowToTransaction(&t)
	}
	if err := r.loadGoalProgress(ctx, tx); err != nil {
		return nil, err
	}
	return nestReimbursements(tx), nil
}

// loadGoalProgress swaps the transactions' goals for the full ones, with
// their transactions, so the goal progress can be shown.
func (r *Repository) loadGoalProgress(ctx context.Context, txs []models.Transaction) error {
	if !slices.ContainsFunc(txs, func(t models.Transaction) bool { return t.Goal != nil }) {
		return nil
	}

	gls, err := r.GetGoals(ctx)
	if err != nil {
		return err
	}
	byID := make(map[string]*models.Goal, len(gls))
	for i := range gls {
		byID[gls[i].ID] = &gls[i]
	}

	for i := range txs {
		if txs[i].Goal == nil {
			continue
		}
		if gl, ok := byID[txs[i].Goal.ID]; ok {
			txs[i].Goal = gl
		}
	}
	return nil
}

// nestReimbursements moves the reimbursements into their expense's
// Reimbursements (oldest first), keeping the order of the rest.
func nestReimbursements(txs []models.Transaction) []models.Transaction {
	idx := make(map[string]int, len(txs))
	for i, t := range txs {
		if !t.IsReimbursement() {
			idx[t.ID] = i
		}
	}

	// Transactions are sorted newest first, walk backwards for oldest first.
	nested := make(map[int]bool)
	for i := len(txs) - 1; i >= 0; i-- {
		if !txs[i].IsReimbursement() {
			continue
		}
		if p, ok := idx[txs[i].ReimbursesID]; ok {
			txs[p].Reimbursements = append(txs[p].Reimbursements, txs[i])
			nested[i] = true
		}
	}

	res := make([]models.Transaction, 0, len(txs)-len(nested))
	for i, t := range txs {
		if !nested[i] {
			res = append(res, t)
		}
	}
	return res
}

func (r *Repository) GetTransaction(ctx context.Context, id string) (*models.Transaction, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbTx, err := q.GetTransaction(ctx, id)
	if err != nil {
		l.Error("querying transaction", zap.Error(err))
		return nil, err
	}
	l.Info("queried transaction")
	tx := dbTransactionRowToTransaction(&dbTx)

	if tx.Goal != nil {
		gl, err := r.GetGoal(ctx, tx.Goal.ID)
		if err != nil {
			l.Error("querying transaction goal", zap.Error(err), zap.String("id", id))
			return nil, err
		}
		tx.Goal = gl
	}

	if tx.Reimbursed > 0 {
		rows, err := q.GetReimbursementsOf(ctx, sql.NullString{String: id, Valid: true})
		if err != nil {
			l.Error("querying reimbursements", zap.Error(err), zap.String("id", id))
			return nil, err
		}
		for _, row := range rows {
			tx.Reimbursements = append(tx.Reimbursements, dbReimbursementRowToTransaction(&row))
		}
	}
	return tx, nil
}

func (r *Repository) DeleteTransaction(ctx context.Context, tx *models.Transaction) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		l := logger.FromCtx(ctx)
		if err := qtx.DeleteTransaction(ctx, tx.ID); err != nil {
			l.Error("deleting transaction", zap.Error(err), zap.String("id", tx.ID))
			return err
		}
		l.Debug("deleted transaction", zap.String("id", tx.ID))

		if !tx.FulfilledAt.IsZero() {
			if err := r.undoTxToAccountWithTxn(qtx, ctx, tx); err != nil {
				l.Error("undoing transaction after deletion", zap.Error(err))
				return err
			}
			l.Debug("undone transaction after deletion")
		}
		return nil
	})
}

func (r *Repository) UpdateTransaction(ctx context.Context, oldTx *models.Transaction, tx *models.Transaction) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		l := logger.FromCtx(ctx)

		var catID, cpID, toAccID, goalID sql.NullString

		if tx.Category != nil {
			catID.Valid = tx.Category.ID != ""
			catID.String = tx.Category.ID
		}
		if tx.Counterparty != nil {
			cpID.Valid = tx.Counterparty.ID != ""
			cpID.String = tx.Counterparty.ID
		}
		if tx.ToAccount != nil {
			toAccID.Valid = tx.ToAccount.ID != ""
			toAccID.String = tx.ToAccount.ID
		}
		if tx.Goal != nil {
			goalID.Valid = tx.Goal.ID != ""
			goalID.String = tx.Goal.ID
		}

		if err := qtx.UpdateTransaction(ctx, queries.UpdateTransactionParams{
			ID:             tx.ID,
			AccountID:      tx.Account.ID,
			Amount:         tx.Amount,
			Kind:           int64(tx.Kind),
			CategoryID:     catID,
			CounterpartyID: cpID,
			ToAccountID:    toAccID,
			ToAmount:       sql.NullFloat64{Float64: tx.ToAmount, Valid: tx.ToAmount != 0},
			GoalID:         goalID,
			Description:    sql.NullString{String: tx.Description, Valid: tx.Description != ""},
			ScheduledTo:    tx.ScheduledTo,
		}); err != nil {
			l.Error("updating transaction", zap.Error(err), zap.Any("new_transaction", tx), zap.Any("old_transaction", oldTx))
			return err
		}
		l.Debug("updated transaction", zap.Any("new_transaction", tx), zap.Any("old_transaction", oldTx))

		if err := r.syncReimbursementsOnUpdateWithTxn(qtx, ctx, oldTx, tx); err != nil {
			return err
		}

		if !oldTx.FulfilledAt.IsZero() {
			if err := r.undoTxToAccountWithTxn(qtx, ctx, oldTx); err != nil {
				return err
			}
			if err := r.applyTxToAccountWithTxn(qtx, ctx, tx); err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *Repository) FulfillTransaction(ctx context.Context, tx *models.Transaction) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		l := logger.FromCtx(ctx)
		if err := qtx.FulfillTransaction(ctx, tx.ID); err != nil {
			l.Error("fulfilling transaction", zap.Error(err), zap.String("id", tx.ID), zap.Time("fulfilled_on", tx.FulfilledAt))
			return err
		}
		l.Debug("fulfilled transaction", zap.String("id", tx.ID), zap.Time("fulfilled_on", tx.FulfilledAt))
		if err := r.applyTxToAccountWithTxn(qtx, ctx, tx); err != nil {
			l.Error("applying transaction to account", zap.Error(err))
			return err
		}
		l.Debug("applied transaction to account")
		return nil
	})
}

func (r *Repository) MarkTransactionReviewed(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	if err := q.MarkTransactionReviewed(ctx, id); err != nil {
		l.Error("marking transaction as reviewed", zap.Error(err), zap.String("id", id))
		return err
	}
	return nil
}

func dbTransactionToTransaction(
	dbTx *queries.Transaction, acc *queries.Account,
	cur *queries.Currency, toAcc *queries.Account, toCur *queries.Currency,
	cat *queries.Category, cp *queries.Counterparty, gl *queries.Goal,
	glCur *queries.Currency,
) *models.Transaction {
	tx := models.Transaction{
		ID:          dbTx.ID,
		Amount:      dbTx.Amount,
		Kind:        models.TxKind(dbTx.Kind),
		Description: dbTx.Description.String,
		ToAmount:    dbTx.ToAmount.Float64,
		FulfilledAt: dbTx.FulfilledAt.Time,
		ScheduledTo: dbTx.ScheduledTo,
		NeedsReview: dbTx.NeedsReview,
		CreatedAt:   dbTx.CreatedAt,

		ReimbursesID: dbTx.ReimbursesID.String,
	}

	if acc != nil {
		tx.Account = *dbAccountToAccount(acc, 0, cur, nil, nil, false)
	}

	if toAcc != nil {
		tx.ToAccount = dbAccountToAccount(toAcc, 0, toCur, nil, nil, false)
	} else if dbTx.ToAccountID.Valid {
		tx.ToAccount = &models.Account{ID: dbTx.ToAccountID.String}
	}

	if cat != nil {
		tx.Category = dbCategoryToCategory(cat, nil)
	} else if dbTx.CategoryID.Valid {
		tx.Category = &models.Category{ID: dbTx.CategoryID.String}
	}

	if cp != nil {
		tx.Counterparty = dbCounterpartyToCounterparty(cp)
	} else if dbTx.CounterpartyID.Valid {
		tx.Counterparty = &models.Counterparty{ID: dbTx.CounterpartyID.String}
	}

	if gl != nil {
		tx.Goal = dbGoalToGoal(gl, glCur)
		tx.GoalPercentage = tx.Percentage(tx.Goal.Price, tx.Goal.Currency.String)
	} else if dbTx.GoalID.Valid {
		tx.Goal = &models.Goal{ID: dbTx.GoalID.String}
	}

	return &tx
}

func dbTransactionRowToTransaction[T queries.GetTransactionRow | queries.GetTransactionsRow](dbTx *T) *models.Transaction {
	_dbTx := queries.GetTransactionsRow(*dbTx)

	var cp *queries.Counterparty
	if _dbTx.Transaction.CounterpartyID.Valid {
		cp = &queries.Counterparty{ID: _dbTx.Transaction.CounterpartyID.String, Name: _dbTx.CounterpartyName.String, Icon: _dbTx.CounterpartyIcon, Color: _dbTx.CounterpartyColor}
	}

	var gl *queries.Goal
	var glCur *queries.Currency
	if _dbTx.Transaction.GoalID.Valid {
		gl = &queries.Goal{
			ID:          _dbTx.Transaction.GoalID.String,
			Name:        _dbTx.GoalName.String,
			CurrencyID:  _dbTx.GoalCurrencyID.String,
			Price:       _dbTx.GoalPrice.Float64,
			FulfilledAt: _dbTx.GoalFulfilledAt,
		}
		if _dbTx.GoalCurrencyID.Valid {
			glCur = &queries.Currency{
				ID:     _dbTx.GoalCurrencyID.String,
				String: _dbTx.GoalCurrencyString.String,
				Symbol: _dbTx.GoalCurrencySymbol.String,
			}
		}
	}

	var toAcc *queries.Account
	var toAccCurr *queries.Currency
	if _dbTx.ToAccountID.Valid {
		toAcc = &queries.Account{
			ID:                 _dbTx.ToAccountID.String,
			Name:               _dbTx.ToAccountName.String,
			KindID:             _dbTx.ToAccountKindID.String,
			Amount:             _dbTx.ToAccountAmount.Float64,
			CurrencyID:         _dbTx.ToAccountCurrencyID.String,
			CounterpartyID:     _dbTx.ToAccountCurrencyID,
			CardNumber:         _dbTx.ToAccountCardNumber,
			CardLimit:          _dbTx.ToAccountCardLimit.Float64,
			NameOnCard:         _dbTx.ToAccountNameOnCard,
			Iban:               _dbTx.ToAccountIban,
			Note:               _dbTx.ToAccountNote,
			IsArchived:         _dbTx.ToAccountIsArchived.Bool,
			ExcludeFromBalance: _dbTx.ToAccountExcludeFromBalance.Bool,
			ExpirationDate:     _dbTx.ToAccountExpirationDate,
			CreatedAt:          _dbTx.ToAccountCreatedAt.Time,
		}
		if _dbTx.ToAccountCurrencyID.Valid {
			toAccCurr = &queries.Currency{
				ID:     toAcc.CurrencyID,
				String: _dbTx.ToAccountCurrencyString.String,
				Symbol: _dbTx.ToAccountCurrencySymbol.String,
			}
		}

	}

	var cat *queries.Category
	if _dbTx.CategoryID.Valid {
		cat = &queries.Category{
			ID:               _dbTx.CategoryID.String,
			Name:             _dbTx.CategoryName.String,
			BudgetAmount:     _dbTx.CategoryBudgetAmount,
			BudgetCurrencyID: _dbTx.CategoryBudgetCurrencyID,
			CreatedAt:        _dbTx.CategoryCreatedAt.Time,
		}
	}

	tx := dbTransactionToTransaction(
		&_dbTx.Transaction, &_dbTx.Account,
		&_dbTx.Currency, toAcc, toAccCurr,
		cat, cp, gl, glCur)
	tx.SMSBody = _dbTx.SmsBody
	tx.FromSMS = tx.SMSBody != ""
	tx.Reimbursed = _dbTx.Reimbursed
	return tx
}
