// Package reqpath carries the request's URL path to templates, e.g. for the
// navbar to mark the current page.
package reqpath

import "context"

type key struct{}

func With(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, key{}, path)
}

func From(ctx context.Context) string {
	p, _ := ctx.Value(key{}).(string)
	return p
}
