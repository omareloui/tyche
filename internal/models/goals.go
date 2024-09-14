package models

import (
	"github.com/omareloui/financial-planner/internal/db"
)

type Goal struct {
	ID string `validate:"required"`
}

func DBGoalToGoal(dbGoal *db.Goal) *Goal {
	return &Goal{
		ID: dbGoal.ID,
	}
}

func DBGoalsToGoals(dbGoal []db.Goal) []Goal {
	goals := make([]Goal, len(dbGoal))
	for i, cur := range dbGoal {
		goals[i] = *DBGoalToGoal(&cur)
	}
	return goals
}
