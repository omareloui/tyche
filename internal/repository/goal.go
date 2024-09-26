package repository

import (
	"context"
	"slices"
	"time"

	"github.com/omareloui/financial-planner/internal/db"
	"github.com/omareloui/financial-planner/internal/models"
)

func (r *Repository) GetGoals(ctx context.Context) ([]models.Goal, error) {
	dbGoals, err := r.q.GetGoals(ctx)
	if err != nil {
		return nil, err
	}
	return dbGoalRowsToGoals(dbGoals), nil
}

func (r *Repository) GetGoal(ctx context.Context, id string) (*models.Goal, error) {
	dbGls, err := r.q.GetGoal(ctx, id)
	if err != nil {
		return nil, err
	}
	return dbGoalRowToGoal(dbGls), nil
}

func (r *Repository) CreateGoal(ctx context.Context, gl *models.Goal) error {
	return r.transact(func(qtx *db.Queries) error {
		if err := qtx.CreateGoal(ctx, db.CreateGoalParams{
			ID:          gl.ID,
			Title:       gl.Title,
			EgpSubtotal: gl.EgpSubtotal,
			CreatedAt:   gl.CreatedAt,
		}); err != nil {
			return err
		}

		for _, it := range gl.Items {
			if err := qtx.CreateGoalItem(ctx, db.CreateGoalItemParams{
				GoalID: gl.ID,
				ItemID: it.ID,
			}); err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *Repository) UpdateGoal(ctx context.Context, gl *models.Goal) error {
	return r.transact(func(qtx *db.Queries) error {
		if err := qtx.UpdateGoal(ctx, db.UpdateGoalParams{
			ID:          gl.ID,
			Title:       gl.Title,
			EgpSubtotal: gl.EgpSubtotal,
		}); err != nil {
			return err
		}

		if err := qtx.DeleteGoalItems(ctx, gl.ID); err != nil {
			return err
		}

		for _, it := range gl.Items {
			if err := qtx.CreateGoalItem(ctx, db.CreateGoalItemParams{
				GoalID: gl.ID,
				ItemID: it.ID,
			}); err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *Repository) DeleteGoal(ctx context.Context, id string) error {
	return r.q.DeleteGoal(ctx, id)
}

func dbGoalRowToGoal(dbGl []db.GetGoalRow) *models.Goal {
	items := []models.Item{}

	for _, gl := range dbGl {
		if gl.ItemID.Valid {
			items = append(items, models.Item{
				ID:          gl.ItemID.String,
				Name:        gl.ItemName.String,
				Price:       gl.ItemPrice.Float64,
				PurchasedOn: gl.ItemPurchasedOn.Time,
				Currency: models.Currency{
					ID:        gl.ItemCurrencyID.String,
					Enum:      models.CurrencyEnum(gl.ItemCurrency.Int64),
					EgpFxRate: gl.ItemCurrencyFxRate.Float64,
				},
			})
		}
	}

	return &models.Goal{
		ID:          dbGl[0].ID,
		Title:       dbGl[0].Title,
		EgpSubtotal: dbGl[0].EgpSubtotal,
		CreatedAt:   dbGl[0].CreatedAt.(time.Time),
		Items:       items,
	}
}

func dbGoalRowsToGoals(dbGls []db.GetGoalsRow) []models.Goal {
	gls := []models.Goal{}

	for _, dbGl := range dbGls {
		glIdx := slices.IndexFunc(gls, func(gl models.Goal) bool { return gl.ID == dbGl.ID })
		if glIdx == -1 {
			gls = append(gls, models.Goal{
				ID:          dbGl.ID,
				Title:       dbGl.Title,
				EgpSubtotal: dbGl.EgpSubtotal,
				CreatedAt:   dbGl.CreatedAt.(time.Time),
				Items:       []models.Item{},
			})
			glIdx = len(gls) - 1
		}

		if dbGl.ItemID.Valid {
			gls[glIdx].Items = append(gls[glIdx].Items, models.Item{
				ID:          dbGl.ItemID.String,
				Name:        dbGl.ItemName.String,
				Price:       dbGl.ItemPrice.Float64,
				PurchasedOn: dbGl.ItemPurchasedOn.Time,
				Currency: models.Currency{
					ID:        dbGl.ItemCurrencyID.String,
					Enum:      models.CurrencyEnum(dbGl.ItemCurrency.Int64),
					EgpFxRate: dbGl.ItemCurrencyFxRate.Float64,
				},
			})
		}
	}

	return gls
}
