package parser

import (
	"testing"
)

func TestParse(t *testing.T) {
	input := "hello"
	expected := "Parsed: hello"
	result := Parse(input)

	if result != expected {
		t.Errorf("Parse(%q) = %q, expected %q", input, result, expected)
	}
}

func TestOneChar(t *testing.T) {
	t.Run("Success: read one character", func(t *testing.T) {
		input := "abc"
		ctx := NewParsingContext(input)
		p := OneChar('a')

		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if res.Result != 'a' {
			t.Errorf("Incorrect result: expected 'a', got %q", res.Result)
		}

		if string(res.Context.Remaining) != "bc" {
			t.Errorf("Incorrect remaining context: expected \"bc\", got %q", string(res.Context.Remaining))
		}

		if res.Context.Position.Offset != 1 {
			t.Errorf("Incorrect position: expected offset 1, got %d", res.Context.Position.Offset)
		}
	})

	t.Run("Failure: wrong character", func(t *testing.T) {
		input := "abc"
		ctx := NewParsingContext(input)
		p := OneChar('b')

		_, err := p(ctx)
		if err == nil {
			t.Fatal("An error was expected (wrong character), but nil was returned")
		}
	})

	t.Run("Failure: end of string", func(t *testing.T) {
		input := ""
		ctx := NewParsingContext(input)
		p := OneChar('a')

		_, err := p(ctx)
		if err == nil {
			t.Fatal("An error was expected (end of string), but nil was returned")
		}

		if err.Error() != "end of string" {
			t.Errorf("Incorrect error message: expected \"end of string\", got %q", err.Error())
		}
	})
}
