// Package tzctx holds the user's time zone in the context, for templates to
// show times (e.g. when a transaction was fulfilled) in it without importing
// the middlewares.
package tzctx

import (
	"context"
	"time"
)

// Key holds the user's *time.Location.
type Key struct{}

// With is ctx holding loc, ctx itself when loc is nil.
func With(ctx context.Context, loc *time.Location) context.Context {
	if loc == nil {
		return ctx
	}
	return context.WithValue(ctx, Key{}, loc)
}

// In is t in the user's time zone, t as is without one in ctx.
func In(ctx context.Context, t time.Time) time.Time {
	if loc, ok := ctx.Value(Key{}).(*time.Location); ok && !t.IsZero() {
		return t.In(loc)
	}
	return t
}

// Today is today in the user's time zone as the date-only form values parse
// (midnight UTC), UTC's without one in ctx.
func Today(ctx context.Context) time.Time {
	y, m, d := In(ctx, time.Now()).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
