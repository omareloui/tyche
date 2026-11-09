package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/omareloui/tyche/internal/errs"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"github.com/omareloui/tyche/internal/resperr"
	"go.uber.org/zap"
)

var (
	// ErrGoalCurrencyUsed is returned changing the currency of a goal that
	// already has money moved in or out.
	ErrGoalCurrencyUsed = &errs.ValidationError{Errors: errs.ValidationErrors{
		"Currency.ID": errs.ValidationField{Tag: "goal_currency_used"},
	}}
	errGoalNotFound = resperr.New(404, "Goal not found")
)

func (r *Repository) GetGoals(ctx context.Context) ([]models.Goal, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbGoals, err := q.GetGoals(ctx)
	if err != nil {
		l.Error("fetching goals", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched goals")
	return goalsWithHoldings(ctx, q, dbGoals, "")
}

func (r *Repository) GetUnfulfilledGoals(ctx context.Context) ([]models.Goal, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbGoals, err := q.GetUnfulfilledGoals(ctx)
	if err != nil {
		l.Error("fetching unfulfilled goals", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched unfulfilled goals")
	return goalsWithHoldings(ctx, q, dbGoals, "")
}

func (r *Repository) GetGoal(ctx context.Context, id string) (*models.Goal, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	dbGls, err := q.GetGoal(ctx, id)
	if err != nil {
		return nil, err
	}
	gls, err := goalsWithHoldings(ctx, q, dbGls, id)
	if err != nil {
		return nil, err
	}
	if len(gls) == 0 {
		return nil, errGoalNotFound
	}
	return &gls[0], nil
}

// CreateGoal creates the goal with its account, holding the money saved.
func (r *Repository) CreateGoal(ctx context.Context, gl *models.Goal) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		return createGoalWithTxn(ctx, qtx, gl)
	})
}

// CreateGoalFromWish creates the goal saving for the wishlist item, the
// item then saved for (models.WishlistSaving) until the goal decides it.
func (r *Repository) CreateGoalFromWish(ctx context.Context, gl *models.Goal, itemID string) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		if err := createGoalWithTxn(ctx, qtx, gl); err != nil {
			return err
		}
		if err := qtx.LinkWishlistItemToGoal(ctx, queries.LinkWishlistItemToGoalParams{
			ID:     itemID,
			GoalID: sql.NullString{String: gl.ID, Valid: true},
		}); err != nil {
			logger.FromCtx(ctx).Error("linking wish to goal", zap.Error(err), zap.String("wish", itemID))
			return err
		}
		return nil
	})
}

func createGoalWithTxn(ctx context.Context, qtx *queries.Queries, gl *models.Goal) error {
	l := logger.FromCtx(ctx)
	kindID, err := qtx.GetAccountKindID(ctx, int64(models.AccKindGoal))
	if err != nil {
		l.Error("getting the goal account kind", zap.Error(err))
		return err
	}
	name, err := goalAccountNameWithTxn(ctx, qtx, gl.Name, gl.Currency.ID)
	if err != nil {
		return err
	}
	accID, err := createGoalAccountWithTxn(ctx, qtx, kindID, name, gl.Currency.ID)
	if err != nil {
		return err
	}
	gl.Account = &models.Account{ID: accID, Currency: gl.Currency}
	if err := qtx.CreateGoal(ctx, queries.CreateGoalParams{
		ID:         gl.ID,
		Name:       gl.Name,
		Price:      gl.Price,
		CurrencyID: gl.Currency.ID,
		AccountID:  sql.NullString{String: accID, Valid: true},
	}); err != nil {
		l.Error("creating goal", zap.Error(err))
		return err
	}
	return setAccountGoalWithTxn(ctx, qtx, accID, gl.ID)
}

// goalAccountNameWithTxn is the name of the goal's (named name) account of
// the currency of id curID.
func goalAccountNameWithTxn(ctx context.Context, qtx *queries.Queries, name, curID string) (string, error) {
	cur, err := qtx.GetCurrency(ctx, curID)
	if err != nil {
		logger.FromCtx(ctx).Error("getting the goal's currency", zap.Error(err))
		return "", err
	}
	return models.GoalAccountName(name, cur.Currency.String), nil
}

func setAccountGoalWithTxn(ctx context.Context, qtx *queries.Queries, accID, goalID string) error {
	if err := qtx.SetAccountGoal(ctx, queries.SetAccountGoalParams{
		ID:     accID,
		GoalID: sql.NullString{String: goalID, Valid: true},
	}); err != nil {
		logger.FromCtx(ctx).Error("tying account to goal", zap.Error(err), zap.String("goal", goalID))
		return err
	}
	return nil
}

// goalAccountInWithTxn is the goal's account of the currency, its own or a
// holding, created when it has none yet.
func goalAccountInWithTxn(ctx context.Context, qtx *queries.Queries, gl *models.Goal, cur *models.Currency) (*models.Account, error) {
	if a := gl.AccountIn(cur.String); a != nil {
		return a, nil
	}
	kindID, err := qtx.GetAccountKindID(ctx, int64(models.AccKindGoal))
	if err != nil {
		return nil, err
	}
	name := models.GoalAccountName(gl.Name, cur.String)
	id, err := createGoalAccountWithTxn(ctx, qtx, kindID, name, cur.ID)
	if err != nil {
		return nil, err
	}
	if err := setAccountGoalWithTxn(ctx, qtx, id, gl.ID); err != nil {
		return nil, err
	}
	a := models.Account{ID: id, Name: name, Currency: *cur, Kind: models.AccountKind{Enum: models.AccKindGoal}, ExcludeFromBalance: true}
	gl.Holdings = append(gl.Holdings, a)
	return &gl.Holdings[len(gl.Holdings)-1], nil
}

// DepositToGoal puts money from tx's account in the goal: tx goes to the
// goal's account of the same currency (a new holding when it has none), the
// money keeping its currency.
func (r *Repository) DepositToGoal(ctx context.Context, gl *models.Goal, tx *models.Transaction) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		to, err := goalAccountInWithTxn(ctx, qtx, gl, &tx.Account.Currency)
		if err != nil {
			return err
		}
		tx.ToAccount, tx.ToAmount = to, tx.Amount
		return r.createTransactionAndApplyToAccountIfNeededWithTxn(ctx, qtx, tx)
	})
}

// createGoalAccountWithTxn creates a hidden account of a goal, named name,
// returning its id.
func createGoalAccountWithTxn(ctx context.Context, qtx *queries.Queries, kindID, name, currencyID string) (string, error) {
	id := idgen.New()
	if err := qtx.CreateAccount(ctx, queries.CreateAccountParams{
		ID:                 id,
		Name:               name,
		KindID:             kindID,
		CurrencyID:         currencyID,
		ExcludeFromBalance: true,
		SmsRefs:            []byte("[]"),
	}); err != nil {
		logger.FromCtx(ctx).Error("creating goal account", zap.Error(err))
		return "", goalNameErr(err)
	}
	return id, nil
}

// goalNameErr is a duplicate name error when the goal's account name is
// taken (by another goal or an account named like it).
func goalNameErr(err error) error {
	if strings.Contains(err.Error(), "UNIQUE constraint failed: accounts.name") {
		return newDuplicationError("Name")
	}
	return err
}

// UpdateGoal updates the goal and its account. The currency can only change
// while no money went in or out.
func (r *Repository) UpdateGoal(ctx context.Context, gl *models.Goal) error {
	l := logger.FromCtx(ctx)
	prev, err := r.GetGoal(ctx, gl.ID)
	if err != nil {
		return err
	}
	return r.transact(ctx, func(qtx *queries.Queries) error {
		if prev.Currency.ID != gl.Currency.ID {
			n, err := qtx.CountAccountTransactions(ctx, prev.Account.ID)
			if err != nil {
				return err
			}
			if n > 0 || len(prev.Holdings) > 0 {
				return ErrGoalCurrencyUsed
			}
		}
		if err := qtx.UpdateGoal(ctx, queries.UpdateGoalParams{
			ID:         gl.ID,
			Name:       gl.Name,
			Price:      gl.Price,
			CurrencyID: gl.Currency.ID,
		}); err != nil {
			l.Error("updating goal", zap.Error(err))
			return err
		}
		name, err := goalAccountNameWithTxn(ctx, qtx, gl.Name, gl.Currency.ID)
		if err != nil {
			return err
		}
		if err := qtx.UpdateGoalAccount(ctx, queries.UpdateGoalAccountParams{
			ID:         prev.Account.ID,
			Name:       name,
			CurrencyID: gl.Currency.ID,
		}); err != nil {
			l.Error("updating goal account", zap.Error(err))
			return goalNameErr(err)
		}
		for _, h := range prev.Holdings {
			if err := qtx.UpdateGoalAccount(ctx, queries.UpdateGoalAccountParams{
				ID:         h.ID,
				Name:       models.GoalAccountName(gl.Name, h.Currency.String),
				CurrencyID: h.Currency.ID,
			}); err != nil {
				l.Error("updating goal holding", zap.Error(err))
				return goalNameErr(err)
			}
		}
		return nil
	})
}

// SetGoalPlan saves how the goal is saved for, nil clearing it.
func (r *Repository) SetGoalPlan(ctx context.Context, id string, plan *models.GoalPlan) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return setGoalPlanWithTxn(ctx, q, id, plan)
}

func setGoalPlanWithTxn(ctx context.Context, q *queries.Queries, id string, plan *models.GoalPlan) error {
	var b []byte
	if plan != nil {
		var err error
		if b, err = json.Marshal(plan); err != nil {
			return err
		}
	}
	if err := q.SetGoalPlan(ctx, queries.SetGoalPlanParams{ID: id, Plan: b}); err != nil {
		logger.FromCtx(ctx).Error("saving goal plan", zap.Error(err), zap.String("id", id))
		return err
	}
	return nil
}

// FulfillGoal records buying the goal, an expense out of its account, and
// marks it achieved. Its holdings are converted to its currency first
// (conversions, transfers from them to its account). A plan's loan payments
// are scheduled with it, linked to the goal (loan_goal_id).
func (r *Repository) FulfillGoal(ctx context.Context, gl *models.Goal, conversions []models.Transaction, purchase *models.Transaction, loanTxs []models.Transaction) error {
	l := logger.FromCtx(ctx)
	id := gl.ID
	return r.transact(ctx, func(qtx *queries.Queries) error {
		for i := range conversions {
			if err := r.createTransactionAndApplyToAccountIfNeededWithTxn(ctx, qtx, &conversions[i]); err != nil {
				return err
			}
		}
		if purchase != nil {
			if err := r.createTransactionAndApplyToAccountIfNeededWithTxn(ctx, qtx, purchase); err != nil {
				return err
			}
		}
		if len(loanTxs) > 0 {
			ids := make([]string, len(loanTxs))
			for i := range loanTxs {
				if err := r.createTransactionWithTxn(qtx, ctx, &loanTxs[i]); err != nil {
					return err
				}
				ids[i] = loanTxs[i].ID
			}
			b, err := json.Marshal(ids)
			if err != nil {
				return err
			}
			if err := qtx.LinkLoanPayments(ctx, queries.LinkLoanPaymentsParams{GoalID: sql.NullString{String: id, Valid: true}, Ids: b}); err != nil {
				l.Error("linking loan payments", zap.Error(err))
				return err
			}
		}
		if err := qtx.FulfillGoal(ctx, id); err != nil {
			l.Error("fulfilling goal", zap.Error(err))
			return err
		}
		// The wish it saved for is bought, with the purchase (or the loan's
		// first payment, with nothing down).
		txID := ""
		if purchase != nil {
			txID = purchase.ID
		} else if len(loanTxs) > 0 {
			txID = loanTxs[0].ID
		}
		if err := setWishlistItemStatusByGoalWithTxn(ctx, qtx, id, models.WishlistBought, txID); err != nil {
			return err
		}
		l.Debug("fulfilled goal", zap.String("id", id))
		return nil
	})
}

// UnfulfillGoal undoes buying the goal: its purchase is deleted, the money
// going back to its account, then its holdings' conversions, the money going
// back to them, with its loan's payments not paid yet (the paid ones are
// unlinked, kept as plain expenses).
func (r *Repository) UnfulfillGoal(ctx context.Context, gl *models.Goal) error {
	l := logger.FromCtx(ctx)
	id := gl.ID
	return r.transact(ctx, func(qtx *queries.Queries) error {
		unpaid, err := qtx.GetUnpaidLoanPayments(ctx, sql.NullString{String: id, Valid: true})
		if err != nil {
			return err
		}
		for _, txID := range unpaid {
			row, err := qtx.GetTransaction(ctx, txID)
			if err != nil {
				return err
			}
			if err := r.deleteTransactionWithTxn(ctx, qtx, dbTransactionRowToTransaction(&row)); err != nil {
				return err
			}
		}
		// The ones paid stay, as expenses of their own.
		if err := qtx.UnlinkLoanPayments(ctx, sql.NullString{String: id, Valid: true}); err != nil {
			return err
		}
		purchaseID, err := qtx.GetGoalPurchaseID(ctx, id)
		if err == nil {
			row, err := qtx.GetTransaction(ctx, purchaseID)
			if err != nil {
				return err
			}
			if err := r.deleteTransactionWithTxn(ctx, qtx, dbTransactionRowToTransaction(&row)); err != nil {
				return err
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			l.Error("getting goal purchase", zap.Error(err))
			return err
		}
		convIDs, err := qtx.GetGoalConversionIDs(ctx, sql.NullString{String: id, Valid: true})
		if err != nil {
			l.Error("getting goal conversions", zap.Error(err))
			return err
		}
		for _, txID := range convIDs {
			row, err := qtx.GetTransaction(ctx, txID)
			if err != nil {
				return err
			}
			if err := r.deleteTransactionWithTxn(ctx, qtx, dbTransactionRowToTransaction(&row)); err != nil {
				return err
			}
		}
		if err := qtx.UnfulfillGoal(ctx, id); err != nil {
			l.Error("unfulfilling goal", zap.Error(err))
			return err
		}
		if err := setWishlistItemStatusByGoalWithTxn(ctx, qtx, id, models.WishlistSaving, ""); err != nil {
			return err
		}
		l.Debug("unfulfilled goal", zap.String("id", id))
		return nil
	})
}

// DeleteGoal deletes the goal, after moving what's left in it with
// withdraws (one per account with money). Its accounts are soft deleted,
// keeping their transactions' history.
func (r *Repository) DeleteGoal(ctx context.Context, gl *models.Goal, withdraws []models.Transaction) error {
	l := logger.FromCtx(ctx)
	return r.transact(ctx, func(qtx *queries.Queries) error {
		for i := range withdraws {
			if err := r.createTransactionAndApplyToAccountIfNeededWithTxn(ctx, qtx, &withdraws[i]); err != nil {
				return err
			}
		}
		// Given up on: the wish it saved for is let go (its goal_id then
		// cleared by the foreign key).
		if err := setWishlistItemStatusByGoalWithTxn(ctx, qtx, gl.ID, models.WishlistAbandoned, ""); err != nil {
			return err
		}
		if err := qtx.DeleteGoal(ctx, gl.ID); err != nil {
			l.Error("deleting goal", zap.Error(err))
			return err
		}
		if gl.Account != nil {
			if err := qtx.DeleteAccount(ctx, gl.Account.ID); err != nil {
				l.Error("deleting goal account", zap.Error(err))
				return err
			}
		}
		for _, h := range gl.Holdings {
			if err := qtx.DeleteAccount(ctx, h.ID); err != nil {
				l.Error("deleting goal holding", zap.Error(err))
				return err
			}
		}
		return nil
	})
}

func dbGoalToGoal(dbGl *queries.Goal, cur *queries.Currency) *models.Goal {
	gl := models.Goal{
		ID:          dbGl.ID,
		Name:        dbGl.Name,
		Price:       dbGl.Price,
		Currency:    models.Currency{ID: dbGl.CurrencyID},
		FulfilledAt: dbGl.FulfilledAt.Time,
		CreatedAt:   dbGl.CreatedAt,
	}

	if cur != nil {
		gl.Currency = *dbCurrencyToCurrency(cur, false)
	}
	if dbGl.AccountID.Valid {
		gl.Account = &models.Account{ID: dbGl.AccountID.String, Currency: gl.Currency}
	}
	if len(dbGl.Plan) > 0 {
		var plan models.GoalPlan
		if err := json.Unmarshal(dbGl.Plan, &plan); err == nil {
			gl.Plan = &plan
		}
	}

	return &gl
}

// goalsWithHoldings is the goals of the rows with their holdings (of the
// goal of id, every goal's when empty).
func goalsWithHoldings[T queries.GetGoalsRow | queries.GetGoalRow | queries.GetUnfulfilledGoalsRow](ctx context.Context, q *queries.Queries, dbGls []T, id string) ([]models.Goal, error) {
	rows, err := q.GetGoalHoldings(ctx, sql.NullString{String: id, Valid: id != ""})
	if err != nil {
		logger.FromCtx(ctx).Error("fetching goal holdings", zap.Error(err))
		return nil, err
	}
	holdings := map[string][]models.Account{}
	for _, row := range rows {
		a := dbAccountToAccount(&row.Account, &row.Currency, nil, nil, false)
		a.Kind.Enum = models.AccKindGoal
		holdings[row.Account.GoalID.String] = append(holdings[row.Account.GoalID.String], *a)
	}
	return dbGoalRowsToGoals(dbGls, holdings), nil
}

func dbGoalRowsToGoals[T queries.GetGoalsRow | queries.GetGoalRow | queries.GetUnfulfilledGoalsRow](dbGls []T, holdings map[string][]models.Account) []models.Goal {
	gls := []models.Goal{}

	for _, _dbGl := range dbGls {
		dbGl := queries.GetGoalsRow(_dbGl)
		glIdx := slices.IndexFunc(gls, func(gl models.Goal) bool { return gl.ID == dbGl.Goal.ID })
		if glIdx == -1 {
			gl := dbGoalToGoal(&dbGl.Goal, &dbGl.Currency)
			gl.Account = dbAccountToAccount(&dbGl.Account, &dbGl.Currency, nil, nil, false)
			gl.Account.Kind.Enum = models.AccKindGoal
			gl.Holdings = holdings[gl.ID]
			gls = append(gls, *gl)
			glIdx = len(gls) - 1
		}

		if !dbGl.TransactionID.Valid {
			continue
		}
		gl := &gls[glIdx]

		_tx := queries.Transaction{
			ID:             dbGl.TransactionID.String,
			AccountID:      dbGl.TransactionAccountID.String,
			Amount:         dbGl.TransactionAmount.Float64,
			Kind:           dbGl.TransactionKind.Int64,
			CategoryID:     dbGl.TransactionCategoryID,
			CounterpartyID: dbGl.TransactionCounterpartyID,
			Description:    dbGl.TransactionDescription,
			ToAccountID:    dbGl.TransactionToAccountID,
			ToAmount:       dbGl.TransactionToAmount,
			FulfilledAt:    dbGl.TransactionFulfilledAt,
			ScheduledTo:    dbGl.TransactionScheduledTo.Time,
			CreatedAt:      dbGl.TransactionCreatedAt.Time,
			LoanGoalID:     dbGl.TransactionLoanGoalID,
		}

		acc := queries.Account{
			ID:                 dbGl.AccountID.String,
			Name:               dbGl.AccountName.String,
			KindID:             dbGl.AccountKindID.String,
			Amount:             dbGl.AccountAmount.Float64,
			CurrencyID:         dbGl.AccountCurrencyID.String,
			CounterpartyID:     dbGl.AccountCounterpartyID,
			CardNumber:         dbGl.AccountCardNumber,
			CardLimit:          dbGl.AccountCardLimit.Float64,
			NameOnCard:         dbGl.AccountNameOnCard,
			Iban:               dbGl.AccountIban,
			Note:               dbGl.AccountNote,
			IsArchived:         dbGl.AccountIsArchived.Bool,
			ExcludeFromBalance: dbGl.AccountExcludeFromBalance.Bool,
			ExpirationDate:     dbGl.AccountExpirationDate,
			CreatedAt:          dbGl.AccountCreatedAt.Time,
		}

		accCurr := queries.Currency{
			ID:      acc.CurrencyID,
			String:  dbGl.AccountCurrencyString.String,
			Symbol:  dbGl.AccountCurrencySymbol.String,
			InGrams: dbGl.AccountCurrencyInGrams.Bool,
		}

		tx := dbTransactionToTransaction(&_tx, &acc, &accCurr, nil, nil, nil, nil, nil, nil)
		if a := gl.AccountByID(acc.ID); a != nil {
			tx.Account = *a
		}
		if tx.ToAccount != nil {
			if a := gl.AccountByID(tx.ToAccount.ID); a != nil {
				tx.ToAccount = a
			}
		}
		if tx.IsGoalPurchase() && !tx.FulfilledAt.IsZero() {
			gl.Purchased += tx.Amount
		}
		if tx.IsLoanPayment() {
			amount := tx.Amount
			if tx.Account.Currency.String != gl.Currency.String {
				if rate := tx.Account.Currency.FxRate(gl.Currency.String); rate > 0 {
					amount *= rate
				}
			}
			gl.Loan.Payments++
			gl.Loan.Total += amount
			if !tx.FulfilledAt.IsZero() {
				gl.Loan.PaidPayments++
				gl.Loan.Paid += amount
			}
		}
		gl.Transactions = append(gl.Transactions, *tx)
	}

	return gls
}
