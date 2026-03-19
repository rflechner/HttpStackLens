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

// Combine combines two parsers into a new parser that parses the input using both parsers in sequence.
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

// OneChar A parser that matches a single character.
func OneChar(c rune) Parser[rune] {
	return func(context ParsingContext) (ParseResult[rune], error) {
		if context.AtEnd() {
			return ParseResult[rune]{
				Context: context,
			}, errors.New("end of string")
		}
		found := context.Remaining[0]
		if found != c {
			return ParseResult[rune]{
				Context: context,
			}, fmt.Errorf("expected %q, found %q", c, found)
		}
		return ParseResult[rune]{
			Result:  c,
			Context: context.Forward(1),
		}, nil
	}
}

// Satisfy A parser that matches a character if the specified predicate returns true.
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

// AnyChar A parser that matches any character.
func AnyChar() Parser[rune] {
	return Satisfy(func(c rune) bool { return true })
}

// Many A parser that attempts to parse the input using the specified parser, collecting all the results into a slice.
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

// Optional A parser that attempts to parse the input using the specified parser, but returns None if the parser fails.
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
			return ParseResult[T]{
				Context: context,
			}, errors.New("no parsers provided to OrElse")
		}
		errorList := list.New()
		for _, parser := range parsers {
			result, err := parser(context)
			if err == nil {
				return result, nil
			}
			errorList.PushBack(err)
		}

		return ParseResult[T]{
			Context: context,
		}, errors.Join(ListToSlice[error](errorList)...)
	}
}

// StringMatch A parser that matches a specified text as a prefix of the input.
func StringMatch(s string) Parser[string] {
	return func(context ParsingContext) (ParseResult[string], error) {
		if context.AtEnd() {
			return ParseResult[string]{
				Context: context,
			}, errors.New("end of string")
		}
		if len(s) > len(context.Remaining) {
			return ParseResult[string]{
				Context: context,
			}, errors.New("string does not match")
		}
		if string(context.Remaining[0:len(s)]) == s {
			return ParseResult[string]{
				Result:  s,
				Context: context.Forward(len(s)),
			}, nil
		}

		return ParseResult[string]{
			Context: context,
		}, errors.New("string does not match")
	}
}

func UntilText[T any](parser Parser[T], delimiter string, includeDelimiter bool) Parser[T] {
	return func(context ParsingContext) (ParseResult[T], error) {
		index := IndexOf(context.Remaining, delimiter)
		if index < 0 {
			return ParseResult[T]{
				Context: context,
			}, errors.New("delimiter not found")
		}
		target := context.Remaining[0:index]
		result, err := parser(ParsingContext{
			Remaining: target,
			Position:  NewTextPosition(),
		})
		if err != nil {
			return ParseResult[T]{
				Context: context,
			}, err
		}
		if result.Context.AtEnd() == false {
			return ParseResult[T]{
				Context: context,
			}, errors.New("parser did not consume all input up to delimiter")
		}

		if includeDelimiter {
			return ParseResult[T]{
				Context: context.Forward(index + len(delimiter)),
				Result:  result.Result,
			}, nil
		}

		return ParseResult[T]{
			Context: context.Forward(index),
			Result:  result.Result,
		}, nil

	}
}

// Between A parser that parses text between 2 other matched parsings.
func Between[A any, B any, C any](before Parser[A], middle Parser[B], after Parser[C]) Parser[struct {
	Before A
	Middle B
	After  C
}] {
	return func(context ParsingContext) (ParseResult[struct {
		Before A
		Middle B
		After  C
	}], error) {
		beforeResult, err := before(context)
		if err != nil {
			return ParseResult[struct {
				Before A
				Middle B
				After  C
			}]{
				Context: beforeResult.Context,
			}, err
		}
		middleResult, err := middle(beforeResult.Context)
		if err != nil {
			return ParseResult[struct {
				Before A
				Middle B
				After  C
			}]{
				Context: middleResult.Context,
			}, err
		}
		afterResult, err := after(middleResult.Context)
		if err != nil {
			return ParseResult[struct {
				Before A
				Middle B
				After  C
			}]{
				Context: afterResult.Context,
			}, err
		}

		return ParseResult[struct {
			Before A
			Middle B
			After  C
		}]{
			Context: afterResult.Context,
			Result: struct {
				Before A
				Middle B
				After  C
			}{
				Before: beforeResult.Result,
				Middle: middleResult.Result,
				After:  afterResult.Result,
			},
		}, nil
	}
}

func SeparatedBy[A any, B any](parser Parser[A], separator Parser[B], matchTailingSeparator bool) Parser[[]A] {
	return func(context ParsingContext) (ParseResult[[]A], error) {
		l := list.New()
		next := context
		previous := context
		hasTailingSeparator := false

		for !next.AtEnd() {
			result, err := parser(next)
			if err != nil {
				break
			}
			l.PushBack(result.Result)
			previous = next
			next = result.Context

			separatorResult, err := separator(next)
			if err != nil {
				hasTailingSeparator = false
				break
			}
			previous = next
			next = separatorResult.Context
			hasTailingSeparator = true
		}

		if l.Len() == 0 {
			return ParseResult[[]A]{
				Context: context,
			}, errors.New("no items found")
		}

		if !matchTailingSeparator && hasTailingSeparator {
			return ParseResult[[]A]{
				Context: previous,
				Result:  ListToSlice[A](l),
			}, nil
		}
		return ParseResult[[]A]{
			Context: next,
			Result:  ListToSlice[A](l),
		}, nil
	}
}

func Spaces() Parser[[]rune] {
	return Many(Satisfy(func(c rune) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }))
}

func LazyParse[T any](factory func() Parser[T]) Parser[T] {
	return func(context ParsingContext) (ParseResult[T], error) {
		return factory()(context)
	}
}
