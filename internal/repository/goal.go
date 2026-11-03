package repository

import (
	"context"
	"database/sql"
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
	return dbGoalRowsToGoals(dbGoals), nil
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
	return dbGoalRowsToGoals(dbGoals), nil
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
	gls := dbGoalRowsToGoals(dbGls)
	if len(gls) == 0 {
		return nil, errGoalNotFound
	}
	return &gls[0], nil
}

// CreateGoal creates the goal with its account, holding the money saved.
func (r *Repository) CreateGoal(ctx context.Context, gl *models.Goal) error {
	l := logger.FromCtx(ctx)
	return r.transact(ctx, func(qtx *queries.Queries) error {
		kindID, err := qtx.GetAccountKindID(ctx, int64(models.AccKindGoal))
		if err != nil {
			l.Error("getting the goal account kind", zap.Error(err))
			return err
		}
		accID, err := createGoalAccountWithTxn(ctx, qtx, kindID, gl.Name, gl.Currency.ID)
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
		return nil
	})
}

// createGoalAccountWithTxn creates the hidden account of a goal, returning
// its id.
func createGoalAccountWithTxn(ctx context.Context, qtx *queries.Queries, kindID, name, currencyID string) (string, error) {
	id := idgen.New()
	if err := qtx.CreateAccount(ctx, queries.CreateAccountParams{
		ID:                 id,
		Name:               models.GoalAccountName(name),
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
			if n > 0 {
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
		if err := qtx.UpdateGoalAccount(ctx, queries.UpdateGoalAccountParams{
			ID:         prev.Account.ID,
			Name:       models.GoalAccountName(gl.Name),
			CurrencyID: gl.Currency.ID,
		}); err != nil {
			l.Error("updating goal account", zap.Error(err))
			return goalNameErr(err)
		}
		return nil
	})
}

// FulfillGoal records buying the goal, an expense out of its account, and
// marks it achieved.
func (r *Repository) FulfillGoal(ctx context.Context, id string, purchase *models.Transaction) error {
	l := logger.FromCtx(ctx)
	return r.transact(ctx, func(qtx *queries.Queries) error {
		if err := r.createTransactionAndApplyToAccountIfNeededWithTxn(ctx, qtx, purchase); err != nil {
			return err
		}
		if err := qtx.FulfillGoal(ctx, id); err != nil {
			l.Error("fulfilling goal", zap.Error(err))
			return err
		}
		l.Debug("fulfilled goal", zap.String("id", id))
		return nil
	})
}

// UnfulfillGoal undoes buying the goal: its purchase is deleted, the money
// going back to its account.
func (r *Repository) UnfulfillGoal(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx)
	return r.transact(ctx, func(qtx *queries.Queries) error {
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
		if err := qtx.UnfulfillGoal(ctx, id); err != nil {
			l.Error("unfulfilling goal", zap.Error(err))
			return err
		}
		l.Debug("unfulfilled goal", zap.String("id", id))
		return nil
	})
}

// DeleteGoal deletes the goal, after moving what's left in it with withdraw
// (if any). Its account is soft deleted, keeping its transactions' history.
func (r *Repository) DeleteGoal(ctx context.Context, gl *models.Goal, withdraw *models.Transaction) error {
	l := logger.FromCtx(ctx)
	return r.transact(ctx, func(qtx *queries.Queries) error {
		if withdraw != nil {
			if err := r.createTransactionAndApplyToAccountIfNeededWithTxn(ctx, qtx, withdraw); err != nil {
				return err
			}
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

	return &gl
}

func dbGoalRowsToGoals[T queries.GetGoalsRow | queries.GetGoalRow | queries.GetUnfulfilledGoalsRow](dbGls []T) []models.Goal {
	gls := []models.Goal{}

	for _, _dbGl := range dbGls {
		dbGl := queries.GetGoalsRow(_dbGl)
		glIdx := slices.IndexFunc(gls, func(gl models.Goal) bool { return gl.ID == dbGl.Goal.ID })
		if glIdx == -1 {
			gl := dbGoalToGoal(&dbGl.Goal, &dbGl.Currency)
			gl.Account = dbAccountToAccount(&dbGl.Account, &dbGl.Currency, nil, nil, false)
			gl.Account.Kind.Enum = models.AccKindGoal
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
		if acc.ID == gl.Account.ID {
			tx.Account = *gl.Account
		}
		if tx.ToAccount != nil && tx.ToAccount.ID == gl.Account.ID {
			tx.ToAccount = gl.Account
		}
		if tx.IsGoalPurchase() && !tx.FulfilledAt.IsZero() {
			gl.Purchased += tx.Amount
		}
		gl.Transactions = append(gl.Transactions, *tx)
	}

	return gls
}
