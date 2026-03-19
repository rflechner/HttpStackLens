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
