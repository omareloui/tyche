package slicesutils

import (
	"slices"
)

func Find[S ~[]E, E any](s S, fun func(E) bool) *E {
	idx := slices.IndexFunc(s, fun)
	if idx == -1 {
		return nil
	}
	return &(s[idx])
}

func Filter[S ~[]E, E any](s S, fun func(E) bool) []E {
	a := []E{}
	for _, e := range s {
		if inc := fun(e); inc {
			a = append(a, e)
		}
	}
	return a
}

func FilterNewItems[T any, K comparable](
	existing []T,
	seed []T,
	keyFn func(T) K,
) []T {
	existingKeys := make(map[K]struct{})
	for _, item := range existing {
		existingKeys[keyFn(item)] = struct{}{}
	}

	var result []T
	for _, item := range seed {
		if _, exists := existingKeys[keyFn(item)]; !exists {
			result = append(result, item)
		}
	}

	return result
}
