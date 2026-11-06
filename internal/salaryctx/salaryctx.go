// Package salaryctx holds the user's monthly salary in the context, for
// templates to blur the transactions about as big as it with the balances
// without importing the middlewares.
package salaryctx

import (
	"context"

	"github.com/omareloui/tyche/internal/models"
)

// Key holds the user's models.Salary.
type Key struct{}

// With is ctx holding s, ctx itself when it isn't set.
func With(ctx context.Context, s models.Salary) context.Context {
	if s.Amount <= 0 {
		return ctx
	}
	return context.WithValue(ctx, Key{}, s)
}

// IsLike is whether amount (in cur) is about the user's salary or more
// (models.Salary.IsLike), false without one in ctx.
func IsLike(ctx context.Context, amount float64, cur *models.Currency) bool {
	s, ok := ctx.Value(Key{}).(models.Salary)
	return ok && s.IsLike(amount, cur)
}
