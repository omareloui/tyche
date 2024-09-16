package repository

import (
	"context"

	"github.com/omareloui/financial-planner/internal/db"
	"github.com/omareloui/financial-planner/internal/models"
)

func (r *Repository) GetGoals(ctx context.Context) ([]models.Goal, error) {
	dbGoals, err := r.q.GetGoals(ctx)
	if err != nil {
		return nil, err
	}
	return models.DBGoalRowsToGoals(dbGoals), nil
}

func (r *Repository) GetGoal(ctx context.Context, id string) (*models.Goal, error) {
	dbGls, err := r.q.GetGoal(ctx, id)
	if err != nil {
		return nil, err
	}
	return models.DBGoalRowToGoal(dbGls), nil
}

func (r *Repository) CreateGoal(ctx context.Context, gl *models.Goal) error {
	txn, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	qtx := r.q.WithTx(txn)

	if err = qtx.CreateGoal(ctx, db.CreateGoalParams{
		ID:          gl.ID,
		Title:       gl.Title,
		EgpSubtotal: gl.EgpSubtotal,
		CreatedAt:   gl.CreatedAt,
	}); err != nil {
		return err
	}

	for _, it := range gl.Items {
		if err = qtx.CreateGoalItem(ctx, db.CreateGoalItemParams{
			GoalID: gl.ID,
			ItemID: it.ID,
		}); err != nil {
			return err
		}
	}

	return txn.Commit()
}

func (r *Repository) UpdateGoal(ctx context.Context, gl *models.Goal) error {
	txn, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	qtx := r.q.WithTx(txn)

	if err = qtx.UpdateGoal(ctx, db.UpdateGoalParams{
		ID:          gl.ID,
		Title:       gl.Title,
		EgpSubtotal: gl.EgpSubtotal,
	}); err != nil {
		return err
	}

	if err = qtx.DeleteGoalItems(ctx, gl.ID); err != nil {
		return err
	}

	for _, it := range gl.Items {
		if err = qtx.CreateGoalItem(ctx, db.CreateGoalItemParams{
			GoalID: gl.ID,
			ItemID: it.ID,
		}); err != nil {
			return err
		}
	}

	return txn.Commit()
}

func (r *Repository) DeleteGoal(ctx context.Context, id string) error {
	return r.q.DeleteGoal(ctx, id)
}
