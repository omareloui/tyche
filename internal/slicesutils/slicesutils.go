package slicesutils

import "slices"

func Find[S ~[]E, E any](s S, fun func(E) bool) *E {
	idx := slices.IndexFunc(s, fun)
	if idx == -1 {
		return nil
	}
	return &(s[idx])
}
