// Package vaultctx holds the key of the tenant's encrypted database in the
// context, for templates to tell whether it's unlocked (e.g. the navbar's
// lock button) without importing the middlewares.
package vaultctx

import "context"

// Key holds the key of the tenant's encrypted database.
type Key struct{}

// Unlocked reports whether the request's database is encrypted and unlocked.
func Unlocked(ctx context.Context) bool {
	k, _ := ctx.Value(Key{}).([]byte)
	return len(k) > 0
}
