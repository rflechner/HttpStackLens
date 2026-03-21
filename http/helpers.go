package http

import (
	"container/list"
	"slices"
)

func ListToSlice[T any](l *list.List) []T {
	result := make([]T, 0, l.Len())
	for e := l.Front(); e != nil; e = e.Next() {
		result = append(result, e.Value.(T)) // type assertion
	}
	return result
}

func IndexOf(remaining []rune, target string) int {
	targetRunes := []rune(target)
	targetLen := len(targetRunes)

	for i := range remaining {
		if i+targetLen > len(remaining) {
			break
		}
		if slices.Equal(remaining[i:i+targetLen], targetRunes) {
			return i
		}
	}
	return -1
}
