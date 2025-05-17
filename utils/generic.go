package utils

import "slices"

func InSlice[T comparable](item T, slice []T) bool {
	return slices.Contains(slice, item)
}
