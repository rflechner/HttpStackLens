package parser

import (
	"errors"
	"fmt"
)

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
