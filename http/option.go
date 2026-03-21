package http

type Option[T any] struct {
	value    T
	hasValue bool
}

func Some[T any](v T) Option[T] {
	return Option[T]{value: v, hasValue: true}
}

func None[T any]() Option[T] {
	return Option[T]{}
}

func (o Option[T]) IsSome() bool { return o.hasValue }
func (o Option[T]) IsNone() bool { return !o.hasValue }
func (o Option[T]) Unwrap() T {
	if !o.hasValue {
		panic("unwrap on None")
	}
	return o.value
}
func (o Option[T]) UnwrapOrDefault(def T) T {
	if o.hasValue {
		return o.value
	}
	return def
}
