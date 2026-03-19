package parser

import (
	"errors"
	"fmt"
)
import "container/list"

type ParseResult[T any] struct {
	Result  T
	Context ParsingContext
}

type Parser[T any] func(context ParsingContext) (ParseResult[T], error)

// Parse analyzes a string and returns a result
func Parse(input string) string {
	return "Parsed: " + input
}

func Combine[A any, B any](left Parser[A], right Parser[B]) Parser[struct {
	Left  A
	Right B
}] {
	return func(context ParsingContext) (ParseResult[struct {
		Left  A
		Right B
	}], error) {
		leftResult, err := left(context)
		if err != nil {
			return ParseResult[struct {
				Left  A
				Right B
			}]{}, err
		}

		rightResult, err := right(leftResult.Context)
		if err != nil {
			return ParseResult[struct {
				Left  A
				Right B
			}]{}, err
		}

		return ParseResult[struct {
			Left  A
			Right B
		}]{
			Result: struct {
				Left  A
				Right B
			}{
				Left:  leftResult.Result,
				Right: rightResult.Result,
			},
			Context: rightResult.Context,
		}, nil
	}
}

func OneChar(c rune) Parser[rune] {
	return func(context ParsingContext) (ParseResult[rune], error) {
		if context.AtEnd() {
			return ParseResult[rune]{}, errors.New("end of string")
		}
		found := context.Remaining[0]
		if found != c {
			return ParseResult[rune]{}, fmt.Errorf("expected %q, found %q", c, found)
		}
		return ParseResult[rune]{
			Result:  c,
			Context: context.Forward(1),
		}, nil
	}
}

func Satisfy(predicate func(rune) bool) Parser[rune] {
	return func(context ParsingContext) (ParseResult[rune], error) {
		if context.AtEnd() {
			return ParseResult[rune]{}, errors.New("end of string")
		}
		c := context.Remaining[0]
		if predicate(c) == false {
			return ParseResult[rune]{}, fmt.Errorf("%q is not expected", c)
		}
		return ParseResult[rune]{
			Result:  c,
			Context: context.Forward(1),
		}, nil
	}
}

func AnyChar() Parser[rune] {
	return Satisfy(func(c rune) bool { return true })
}

func Many[T any](parser Parser[T]) Parser[[]T] {
	return func(context ParsingContext) (ParseResult[[]T], error) {
		l := list.New()
		next := context

		for !next.AtEnd() {
			result, err := parser(next)
			if err != nil {
				break
			}
			l.PushBack(result.Result)
			next = result.Context
		}
		items := ListToSlice[T](l)
		return ParseResult[[]T]{
			Result:  items,
			Context: next,
		}, nil
	}
}

func Optional[T any](parser Parser[T]) Parser[Option[T]] {
	return func(context ParsingContext) (ParseResult[Option[T]], error) {
		result, err := parser(context)
		if err != nil {
			return ParseResult[Option[T]]{
				Result:  None[T](),
				Context: context,
			}, nil
		}
		return ParseResult[Option[T]]{
			Result:  Some(result.Result),
			Context: result.Context,
		}, nil
	}
}

// OrElse A parser that attempts to parse the input using multiple parsers in sequence until one succeeds.
func OrElse[T any](parsers ...Parser[T]) Parser[T] {
	return func(context ParsingContext) (ParseResult[T], error) {
		if len(parsers) == 0 {
			return ParseResult[T]{}, errors.New("no parsers provided to OrElse")
		}
		errorList := list.New()
		for _, parser := range parsers {
			result, err := parser(context)
			if err == nil {
				return result, nil
			}
			errorList.PushBack(err)
		}

		return ParseResult[T]{}, errors.Join(ListToSlice[error](errorList)...)
	}
}
