package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

// CreateTransaction creates tx and the scheduledTxs, and rt, tx being its
// first occurrence, with its next ones through the schedule horizon.
func (r *Repository) CreateTransaction(ctx context.Context, tx *models.Transaction, rt *models.RecurrentTx, scheduledTxs []models.Transaction) error {
	var today time.Time
	if rt != nil {
		today = r.userToday(ctx)
	}
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

		if rt != nil {
			return r.scheduleRecurrentTxWithTxn(ctx, qtx, rt, today)
		}
		return nil
	})
}

func (r *Repository) createTransactionWithTxn(qtx *queries.Queries, ctx context.Context, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)

	var catID, cpID, toAccID, recurrentID sql.NullString

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

// txPageSize is about how many fulfilled transactions a chunk of the list
// has, rounded up to whole months so their totals stay right.
const txPageSize = 50

// GetTransactions is every transaction, unfiltered, newest first.
func (r *Repository) GetTransactions(ctx context.Context) ([]models.Transaction, error) {
	all := time.Time{}
	p, err := r.GetTransactionsPage(ctx, models.TxFilter{}, time.Time{}, &all)
	if err != nil {
		return nil, err
	}
	// Upcoming are soonest first, put them back newest first.
	slices.Reverse(p.Upcoming)
	return append(p.Upcoming, p.History...), nil
}

// GetTransactionsPage is a chunk of the filtered transactions list: the
// first one (with the upcoming transactions) for a zero before, else the one
// older than before. A non-nil until re-renders the list as loaded so far,
// from now back to *until (zero for everything), as after changing a
// transaction.
func (r *Repository) GetTransactionsPage(ctx context.Context, f models.TxFilter, before time.Time, until *time.Time) (*models.TxPage, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	page := &models.TxPage{MonthStart: monthStartFromCtx(ctx)}
	list := func(p queries.GetTransactionsParams) ([]models.Transaction, error) {
		return r.listTransactions(ctx, p)
	}

	base := txFilterParams(&f)
	if before.IsZero() && f.Status != models.TxStatusFulfilled {
		p := base
		p.Fulfilled = false
		if page.Upcoming, err = list(p); err != nil {
			return nil, err
		}
		// Soonest first.
		slices.Reverse(page.Upcoming)
	}

	if f.Status != models.TxStatusScheduled {
		p := base
		p.Fulfilled = true
		if !before.IsZero() {
			p.Before = before
		}

		var after time.Time
		if until != nil {
			after = *until
		} else {
			cut := p
			cut.Lim, cut.Off = 1, txPageSize-1
			rows, err := list(cut)
			if err != nil {
				return nil, err
			}
			if len(rows) > 0 {
				after = page.MonthStart.Of(rows[0].ScheduledTo.UTC()).From
			}
		}

		if !after.IsZero() {
			p.After = after
		}
		if page.History, err = list(p); err != nil {
			return nil, err
		}

		if !after.IsZero() {
			older := base
			older.Fulfilled = true
			older.Before = after
			older.Lim = 1
			rows, err := list(older)
			if err != nil {
				return nil, err
			}
			page.HasMore = len(rows) > 0
			page.Until = after
		}
	}

	if err := r.loadReimbursements(ctx, q, page.Upcoming, page.History); err != nil {
		return nil, err
	}
	if err := r.loadTransferPairs(ctx, q, page.Upcoming, page.History); err != nil {
		return nil, err
	}
	if err := r.loadGoalProgress(ctx, page.Upcoming); err != nil {
		return nil, err
	}
	if err := r.loadGoalProgress(ctx, page.History); err != nil {
		return nil, err
	}

	n, err := q.CountTransactionsNeedingReview(ctx)
	if err != nil {
		l.Error("counting transactions to review", zap.Error(err))
		return nil, err
	}
	page.NeedsReview = int(n)

	l.Info("queried transactions page",
		zap.Int("upcoming", len(page.Upcoming)), zap.Int("history", len(page.History)),
		zap.Time("until", page.Until), zap.Bool("has_more", page.HasMore))
	return page, nil
}

// GetTransactionsSummary is what the filter matches as a whole, in def.
func (r *Repository) GetTransactionsSummary(ctx context.Context, f models.TxFilter, def *models.Currency) (*models.TxSummary, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	lp := txFilterParams(&f)
	p := queries.GetTransactionsSummaryParams{
		Q: lp.Q, Kind: lp.Kind, AccountID: lp.AccountID, CategoryID: lp.CategoryID,
		CounterpartyID: lp.CounterpartyID, GoalID: lp.GoalID, NeedsReview: lp.NeedsReview,
		FromSms: lp.FromSms, FromDate: lp.FromDate, ToDate: lp.ToDate,
		MinAmount: lp.MinAmount, MaxAmount: lp.MaxAmount,
	}
	switch f.Status {
	case models.TxStatusFulfilled:
		p.Fulfilled = true
	case models.TxStatusScheduled:
		p.Fulfilled = false
	}

	rows, err := q.GetTransactionsSummary(ctx, p)
	if err != nil {
		logger.FromCtx(ctx).Error("querying transactions summary", zap.Error(err))
		return nil, err
	}

	s := &models.TxSummary{}
	for _, row := range rows {
		s.Count += int(row.Count)
		if !row.Fulfilled {
			s.Scheduled += int(row.Count)
			continue
		}
		cur := dbCurrencyToCurrency(&row.Currency, false)
		in, okIn := models.Convert(row.Income, cur, def.String)
		out, okOut := models.Convert(row.Expenses, cur, def.String)
		if !okIn || !okOut {
			s.Partial = true
			continue
		}
		s.In += in
		s.Out += out
	}
	return s, nil
}

// txFilterParams is the list query for the filter, all of it (no limit).
func txFilterParams(f *models.TxFilter) queries.GetTransactionsParams {
	p := queries.GetTransactionsParams{Lim: -1}
	if f.Q != "" {
		p.Q = strings.ToLower(f.Q)
	}
	if f.Kind != models.NilTxKind {
		p.Kind = int64(f.Kind)
	}
	if f.AccountID != "" {
		p.AccountID = f.AccountID
	}
	if f.CategoryID != "" {
		p.CategoryID = f.CategoryID
	}
	if f.CounterpartyID != "" {
		p.CounterpartyID = f.CounterpartyID
	}
	if f.GoalID != "" {
		p.GoalID = f.GoalID
	}
	if f.Status == models.TxStatusReview {
		p.NeedsReview = true
	}
	switch f.Source {
	case models.TxSourceSMS:
		p.FromSms = true
	case models.TxSourceManual:
		p.FromSms = false
	}
	if !f.From.IsZero() {
		p.FromDate = f.From.UTC()
	}
	if !f.To.IsZero() {
		// Up to the end of the day: the next one, exclusive, as transactions
		// may have a time of day (e.g. from SMS).
		p.ToDate = f.To.UTC().AddDate(0, 0, 1)
	}
	if f.MinAmount != nil {
		p.MinAmount = *f.MinAmount
	}
	if f.MaxAmount != nil {
		p.MaxAmount = *f.MaxAmount
	}
	return p
}

// loadReimbursements nests the reimbursements of the expenses in lists under
// them, oldest first. They're left out of the list query, so an expense gets
// them even when they're in another chunk.
func (r *Repository) loadReimbursements(ctx context.Context, q *queries.Queries, lists ...[]models.Transaction) error {
	var ids []string
	for _, txs := range lists {
		for _, t := range txs {
			if t.Reimbursed > 0 {
				ids = append(ids, t.ID)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}

	b, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	rows, err := q.GetReimbursementsOfMany(ctx, string(b))
	if err != nil {
		logger.FromCtx(ctx).Error("querying reimbursements", zap.Error(err))
		return err
	}

	byExpense := make(map[string][]models.Transaction)
	// Rows are newest first, walk backwards for oldest first.
	for i := len(rows) - 1; i >= 0; i-- {
		rt := dbTransactionRowToTransaction(&rows[i])
		byExpense[rt.ReimbursesID] = append(byExpense[rt.ReimbursesID], *rt)
	}
	for _, txs := range lists {
		for i := range txs {
			txs[i].Reimbursements = byExpense[txs[i].ID]
		}
	}
	return nil
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
	one := []models.Transaction{*dbTransactionRowToTransaction(&dbTx)}
	if err := markAutoRecurrent(ctx, q, one); err != nil {
		return nil, err
	}
	tx := &one[0]

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

	if tx.IsTransferLeg() && tx.Kind == models.ExpenseTxKind {
		txs := []models.Transaction{*tx}
		if err := r.loadTransferPairs(ctx, q, txs); err != nil {
			return nil, err
		}
		tx.TransferPair = txs[0].TransferPair
	}
	return tx, nil
}

func (r *Repository) DeleteTransaction(ctx context.Context, tx *models.Transaction) error {
	return r.DeleteTransactions(ctx, []*models.Transaction{tx})
}

// DeleteTransactions deletes txs at once, all or none.
func (r *Repository) DeleteTransactions(ctx context.Context, txs []*models.Transaction) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		for _, tx := range txs {
			if err := r.deleteTransactionWithTxn(ctx, qtx, tx); err != nil {
				return err
			}
		}
		return nil
	})
}

// deleteTransactionWithTxn deletes tx, undoing it on its accounts if it was
// fulfilled.
func (r *Repository) deleteTransactionWithTxn(ctx context.Context, qtx *queries.Queries, tx *models.Transaction) error {
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
}

func (r *Repository) UpdateTransaction(ctx context.Context, oldTx *models.Transaction, tx *models.Transaction) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		return r.updateTransactionWithTxn(ctx, qtx, oldTx, tx)
	})
}

// BulkUpdateTransactions saves every news[i] over olds[i] at once, all or
// none, marking them reviewed too when asked.
func (r *Repository) BulkUpdateTransactions(ctx context.Context, olds, news []*models.Transaction, reviewed bool) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		for i := range news {
			if err := r.updateTransactionWithTxn(ctx, qtx, olds[i], news[i]); err != nil {
				return err
			}
			if reviewed && olds[i].NeedsReview {
				if err := qtx.MarkTransactionReviewed(ctx, news[i].ID); err != nil {
					logger.FromCtx(ctx).Error("marking transaction as reviewed", zap.Error(err), zap.String("id", news[i].ID))
					return err
				}
			}
		}
		return nil
	})
}

// GetFilteredTransactionIDs is the ids of the transactions the filter matches
// (both legs of a transfer pair), and how many rows the list shows of them,
// up to limit rows (-1 for all).
func (r *Repository) GetFilteredTransactionIDs(ctx context.Context, f models.TxFilter, limit int) ([]string, int, error) {
	p := txFilterParams(&f)
	p.Lim = int64(limit)
	switch f.Status {
	case models.TxStatusFulfilled:
		p.Fulfilled = true
	case models.TxStatusScheduled:
		p.Fulfilled = false
	}
	txs, err := r.listTransactions(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]string, 0, len(txs))
	for _, t := range txs {
		ids = append(ids, t.ID)
		if t.IsTransferLeg() {
			ids = append(ids, t.TransferPairID)
		}
	}
	return ids, len(txs), nil
}

func (r *Repository) updateTransactionWithTxn(ctx context.Context, qtx *queries.Queries, oldTx *models.Transaction, tx *models.Transaction) error {
	l := logger.FromCtx(ctx)

	var catID, cpID, toAccID sql.NullString

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

	if err := qtx.UpdateTransaction(ctx, queries.UpdateTransactionParams{
		ID:             tx.ID,
		AccountID:      tx.Account.ID,
		Amount:         tx.Amount,
		Kind:           int64(tx.Kind),
		CategoryID:     catID,
		CounterpartyID: cpID,
		ToAccountID:    toAccID,
		ToAmount:       sql.NullFloat64{Float64: tx.ToAmount, Valid: tx.ToAmount != 0},
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
	if err := r.syncTransferPairOnUpdateWithTxn(qtx, ctx, oldTx, tx); err != nil {
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

		ReimbursesID:   dbTx.ReimbursesID.String,
		TransferPairID: dbTx.TransferPairID.String,
		RecurrentTxID:  dbTx.RecurrentID.String,
		LoanGoalID:     dbTx.LoanGoalID.String,
	}

	if acc != nil {
		tx.Account = *dbAccountToAccount(acc, cur, nil, nil, false)
	}

	if toAcc != nil {
		tx.ToAccount = dbAccountToAccount(toAcc, toCur, nil, nil, false)
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
	}

	return &tx
}

func dbTransactionRowToTransaction[T queries.GetTransactionRow | queries.GetTransactionsRow | queries.GetReimbursementsOfManyRow | queries.GetTransferPairsOfManyRow | queries.GetTransferPairCandidatesRow](dbTx *T) *models.Transaction {
	_dbTx := queries.GetTransactionsRow(*dbTx)

	var cp *queries.Counterparty
	if _dbTx.Transaction.CounterpartyID.Valid {
		cp = &queries.Counterparty{ID: _dbTx.Transaction.CounterpartyID.String, Name: _dbTx.CounterpartyName.String, Icon: _dbTx.CounterpartyIcon, Color: _dbTx.CounterpartyColor, IsMerchant: _dbTx.CounterpartyIsMerchant}
	}

	var gl *queries.Goal
	var glCur *queries.Currency
	if _dbTx.GoalID.Valid {
		gl = &queries.Goal{
			ID:          _dbTx.GoalID.String,
			Name:        _dbTx.GoalName.String,
			CurrencyID:  _dbTx.GoalCurrencyID.String,
			Price:       _dbTx.GoalPrice.Float64,
			FulfilledAt: _dbTx.GoalFulfilledAt,
		}
		if _dbTx.GoalCurrencyID.Valid {
			glCur = &queries.Currency{
				ID:      _dbTx.GoalCurrencyID.String,
				String:  _dbTx.GoalCurrencyString.String,
				Symbol:  _dbTx.GoalCurrencySymbol.String,
				InGrams: _dbTx.GoalCurrencyInGrams.Bool,
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
			CounterpartyID:     _dbTx.ToAccountCounterpartyID,
			CardNumber:         _dbTx.ToAccountCardNumber,
			CardLimit:          _dbTx.ToAccountCardLimit.Float64,
			NameOnCard:         _dbTx.ToAccountNameOnCard,
			Iban:               _dbTx.ToAccountIban,
			Note:               _dbTx.ToAccountNote,
			IsArchived:         _dbTx.ToAccountIsArchived.Bool,
			ExcludeFromBalance: _dbTx.ToAccountExcludeFromBalance.Bool,
			ExpirationDate:     _dbTx.ToAccountExpirationDate,
			CreatedAt:          _dbTx.ToAccountCreatedAt.Time,
			DeletedAt:          _dbTx.ToAccountDeletedAt,
		}
		if _dbTx.ToAccountCurrencyID.Valid {
			toAccCurr = &queries.Currency{
				ID:      toAcc.CurrencyID,
				String:  _dbTx.ToAccountCurrencyString.String,
				Symbol:  _dbTx.ToAccountCurrencySymbol.String,
				InGrams: _dbTx.ToAccountCurrencyInGrams.Bool,
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
			Color:            _dbTx.CategoryColor.String,
			CreatedAt:        _dbTx.CategoryCreatedAt.Time,
		}
	}

	tx := dbTransactionToTransaction(
		&_dbTx.Transaction, &_dbTx.Account,
		&_dbTx.Currency, toAcc, toAccCurr,
		cat, cp, gl, glCur)
	tx.Account.Kind.Enum = models.AccKind(_dbTx.AccountKindEnum.Int64)
	if tx.ToAccount != nil {
		tx.ToAccount.Kind.Enum = models.AccKind(_dbTx.ToAccountKindEnum.Int64)
	}
	tx.SMSBody = _dbTx.SmsBody
	tx.FromSMS = tx.SMSBody != ""
	tx.Reimbursed = _dbTx.Reimbursed
	return tx
}

func (r *Repository) MarkAllTransactionsReviewed(ctx context.Context) error {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	if err := q.MarkAllTransactionsReviewed(ctx); err != nil {
		l.Error("marking all transactions as reviewed", zap.Error(err))
		return err
	}
	return nil
}

// ReviewTransactionCategory sets a transaction's category (empty clears it)
// and marks it as reviewed.
func (r *Repository) ReviewTransactionCategory(ctx context.Context, id, categoryID string) error {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	if err := q.ReviewTransactionCategory(ctx, queries.ReviewTransactionCategoryParams{
		ID:         id,
		CategoryID: sql.NullString{String: categoryID, Valid: categoryID != ""},
	}); err != nil {
		l.Error("reviewing transaction category", zap.Error(err), zap.String("id", id))
		return err
	}
	return nil
}
