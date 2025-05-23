package repository

import (
	"context"
	"slices"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
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
	return &dbGoalRowsToGoals(dbGls)[0], nil
}

func (r *Repository) CreateGoal(ctx context.Context, gl *models.Goal) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	return q.CreateGoal(ctx, queries.CreateGoalParams{
		ID:         gl.ID,
		Name:       gl.Name,
		Price:      gl.Price,
		CurrencyID: gl.Currency.ID,
	})
}

func (r *Repository) UpdateGoal(ctx context.Context, gl *models.Goal) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	return q.UpdateGoal(ctx, queries.UpdateGoalParams{
		ID:         gl.ID,
		Name:       gl.Name,
		Price:      gl.Price,
		CurrencyID: gl.Currency.ID,
	})
}

func (r *Repository) FulfillGoal(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	if err := q.FulfillGoal(ctx, id); err != nil {
		l.Error("fulfilling goal", zap.Error(err))
		return err
	}
	l.Debug("fulfilled goal", zap.String("id", id))
	return nil
}

func (r *Repository) UnfulfillGoal(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	if err := q.UnfulfillGoal(ctx, id); err != nil {
		l.Error("fulfilling goal", zap.Error(err))
		return err
	}
	l.Debug("fulfilled goal", zap.String("id", id))
	return nil
}

func (r *Repository) DeleteGoal(ctx context.Context, id string) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	return q.DeleteGoal(ctx, id)
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
		gl.Currency = *dbCurrencyToCurrency(cur)
	}

	return &gl
}

func dbGoalRowsToGoals[T queries.GetGoalsRow | queries.GetGoalRow | queries.GetUnfulfilledGoalsRow](dbGls []T) []models.Goal {
	gls := []models.Goal{}

	for _, _dbGl := range dbGls {
		dbGl := queries.GetGoalsRow(_dbGl)
		glIdx := slices.IndexFunc(gls, func(gl models.Goal) bool { return gl.ID == dbGl.Goal.ID })
		if glIdx == -1 {
			gls = append(gls, *dbGoalToGoal(&dbGl.Goal, &dbGl.Currency))
			glIdx = len(gls) - 1
		}

		if !dbGl.TransactionID.Valid {
			continue
		}

		_tx := queries.Transaction{
			ID:             dbGl.TransactionID.String,
			AccountID:      dbGl.TransactionAccountID.String,
			Amount:         dbGl.TransactionAmount.Float64,
			KindID:         dbGl.TransactionKindID.String,
			CategoryID:     dbGl.TransactionCategoryID,
			CounterpartyID: dbGl.TransactionCounterpartyID,
			Description:    dbGl.TransactionDescription,
			ToAccountID:    dbGl.TransactionToAccountID,
			ToAmount:       dbGl.TransactionToAmount,
			GoalID:         dbGl.TransactionGoalID,
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
			CounterpartyID:     dbGl.AccountCurrencyID,
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
			ID:   acc.CurrencyID,
			Enum: dbGl.AccountCurrencyEnum.Int64,
		}

		tx := dbTransactionToTransaction(
			&_tx, nil, &acc,
			&accCurr, nil, nil, nil, nil,
			&dbGl.Goal, &dbGl.Currency)
		tx.GoalPercentage = tx.Percentage(gls[glIdx].Price, gls[glIdx].Currency.Enum)
		gls[glIdx].Transactions = append(gls[glIdx].Transactions, *tx)
	}

	return gls
}
