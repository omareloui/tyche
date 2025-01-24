package repository

import (
	"context"
	"slices"

	"github.com/omareloui/tyche/internal/db"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"go.uber.org/zap"
)

func (r *Repository) GetGoals(ctx context.Context) ([]models.Goal, error) {
	l := logger.FromCtx(ctx)
	dbGoals, err := r.q.GetGoals(ctx)
	if err != nil {
		l.Error("fetching goals", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched goals")
	return dbGoalRowsToGoals(dbGoals), nil
}

func (r *Repository) GetUnfulfilledGoals(ctx context.Context) ([]models.Goal, error) {
	l := logger.FromCtx(ctx)
	dbGoals, err := r.q.GetUnfulfilledGoals(ctx)
	if err != nil {
		l.Error("fetching unfulfilled goals", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched unfulfilled goals")
	return dbGoalUnfulfilledRowsToGoals(dbGoals), nil
}

func (r *Repository) GetGoal(ctx context.Context, id string) (*models.Goal, error) {
	dbGls, err := r.q.GetGoal(ctx, id)
	if err != nil {
		return nil, err
	}
	return dbGoalRowToGoal(dbGls), nil
}

func (r *Repository) CreateGoal(ctx context.Context, gl *models.Goal) error {
	return r.q.CreateGoal(ctx, db.CreateGoalParams{
		ID:         gl.ID,
		Name:       gl.Name,
		Price:      gl.Price,
		CurrencyID: gl.Currency.ID,
	})
}

func (r *Repository) UpdateGoal(ctx context.Context, gl *models.Goal) error {
	return r.q.UpdateGoal(ctx, db.UpdateGoalParams{
		ID:         gl.ID,
		Name:       gl.Name,
		Price:      gl.Price,
		CurrencyID: gl.Currency.ID,
	})
}

func (r *Repository) FulfillGoal(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx)
	if err := r.q.FulfillGoal(ctx, id); err != nil {
		l.Error("fulfilling goal", zap.Error(err))
		return err
	}
	l.Debug("fulfilled goal", zap.String("id", id))
	return nil
}

func (r *Repository) UnfulfillGoal(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx)
	if err := r.q.UnfulfillGoal(ctx, id); err != nil {
		l.Error("fulfilling goal", zap.Error(err))
		return err
	}
	l.Debug("fulfilled goal", zap.String("id", id))
	return nil
}

func (r *Repository) DeleteGoal(ctx context.Context, id string) error {
	return r.q.DeleteGoal(ctx, id)
}

func dbGoalRowToGoal(dbGl []db.GetGoalRow) *models.Goal {
	gl := models.Goal{
		ID:           dbGl[0].ID,
		Name:         dbGl[0].Name,
		Price:        dbGl[0].Price,
		Currency:     models.Currency{ID: dbGl[0].CurrencyID, Enum: models.CurrencyEnum(dbGl[0].Currency.Int64)},
		FulfilledAt:  dbGl[0].FulfilledAt.Time,
		CreatedAt:    dbGl[0].CreatedAt,
		Transactions: []models.Transaction{},
	}

	for _, g := range dbGl {
		if g.TransactionID.Valid {
			tx := models.Transaction{
				ID:     g.TransactionID.String,
				Amount: g.TransactionAmount.Float64,
				Account: models.Account{
					ID:   g.TransactionAccountID.String,
					Name: g.TransactionAccountName.String,
					Currency: models.Currency{
						ID:   g.TransactionAccountCurrencyID.String,
						Enum: models.CurrencyEnum(g.TransactionAccountCurrencyEnum.Int64),
					},
				},
				ScheduledTo: g.TransactionScheduledTo.Time,
				FulfilledAt: g.TransactionFulfilledAt.Time,
			}

			tx.GoalPercentage = tx.Percentage(gl.Price, gl.Currency.Enum)
			gl.Transactions = append(gl.Transactions, tx)
		}
	}

	return &gl
}

func dbGoalRowsToGoals(dbGls []db.GetGoalsRow) []models.Goal {
	gls := []models.Goal{}

	for _, dbGl := range dbGls {
		glIdx := slices.IndexFunc(gls, func(gl models.Goal) bool { return gl.ID == dbGl.ID })
		if glIdx == -1 {
			gls = append(gls, models.Goal{
				ID:           dbGl.ID,
				Name:         dbGl.Name,
				Price:        dbGl.Price,
				Currency:     models.Currency{ID: dbGl.CurrencyID, Enum: models.CurrencyEnum(dbGl.Currency.Int64)},
				FulfilledAt:  dbGl.FulfilledAt.Time,
				CreatedAt:    dbGl.CreatedAt,
				Transactions: []models.Transaction{},
			})
			glIdx = len(gls) - 1
		}

		if dbGl.TransactionID.Valid {
			if idx := slices.IndexFunc(gls[glIdx].Transactions, func(tx models.Transaction) bool { return tx.ID == dbGl.TransactionID.String }); idx == -1 {
				tx := models.Transaction{
					ID:     dbGl.TransactionID.String,
					Amount: dbGl.TransactionAmount.Float64,
					Account: models.Account{
						ID:   dbGl.TransactionAccountID.String,
						Name: dbGl.TransactionAccountName.String,
						Currency: models.Currency{
							ID:   dbGl.TransactionAccountCurrencyID.String,
							Enum: models.CurrencyEnum(dbGl.TransactionAccountCurrencyEnum.Int64),
						},
					},
					ScheduledTo: dbGl.TransactionScheduledTo.Time,
					FulfilledAt: dbGl.TransactionFulfilledAt.Time,
				}
				tx.GoalPercentage = tx.Percentage(dbGl.Price, models.CurrencyEnum(dbGl.Currency.Int64))
				gls[glIdx].Transactions = append(gls[glIdx].Transactions, tx)
			}
		}
	}

	return gls
}

func dbGoalUnfulfilledRowsToGoals(dbGls []db.GetUnfulfilledGoalsRow) []models.Goal {
	gls := []models.Goal{}

	for _, dbGl := range dbGls {
		glIdx := slices.IndexFunc(gls, func(gl models.Goal) bool { return gl.ID == dbGl.ID })
		if glIdx == -1 {
			gls = append(gls, models.Goal{
				ID:           dbGl.ID,
				Name:         dbGl.Name,
				Price:        dbGl.Price,
				Currency:     models.Currency{ID: dbGl.CurrencyID, Enum: models.CurrencyEnum(dbGl.Currency.Int64)},
				FulfilledAt:  dbGl.FulfilledAt.Time,
				CreatedAt:    dbGl.CreatedAt,
				Transactions: []models.Transaction{},
			})
			glIdx = len(gls) - 1
		}

		if dbGl.TransactionID.Valid {
			if idx := slices.IndexFunc(gls[glIdx].Transactions, func(tx models.Transaction) bool { return tx.ID == dbGl.TransactionID.String }); idx == -1 {
				tx := models.Transaction{
					ID:     dbGl.TransactionID.String,
					Amount: dbGl.TransactionAmount.Float64,
					Account: models.Account{
						ID:   dbGl.TransactionAccountID.String,
						Name: dbGl.TransactionAccountName.String,
						Currency: models.Currency{
							ID:   dbGl.TransactionAccountCurrencyID.String,
							Enum: models.CurrencyEnum(dbGl.TransactionAccountCurrencyEnum.Int64),
						},
					},
					ScheduledTo: dbGl.TransactionScheduledTo.Time,
					FulfilledAt: dbGl.TransactionFulfilledAt.Time,
				}
				tx.GoalPercentage = tx.Percentage(dbGl.Price, models.CurrencyEnum(dbGl.Currency.Int64))
				gls[glIdx].Transactions = append(gls[glIdx].Transactions, tx)
			}
		}
	}

	return gls
}
