package parser

import "container/list"

func ListToSlice[T any](l *list.List) []T {
	result := make([]T, 0, l.Len())
	for e := l.Front(); e != nil; e = e.Next() {
		result = append(result, e.Value.(T)) // type assertion
	}
	return result
}
