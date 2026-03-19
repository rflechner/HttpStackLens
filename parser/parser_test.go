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

func TestCombine(t *testing.T) {

	t.Run("Success: combine two parsers of one char", func(t *testing.T) {
		input := "abc"
		ctx := NewParsingContext(input)
		p := Combine(OneChar('a'), OneChar('b'))
		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		t.Logf("Combined result: %+v", res)
		if res.Context.Position.Offset != 2 {
			t.Errorf("Incorrect position: expected offset 2, got %d", res.Context.Position.Offset)
		}
		if res.Context.Position.Line != 1 {
			t.Errorf("Incorrect position: expected line 1, got %d", res.Context.Position.Line)
		}
		if res.Context.Position.Column != 3 {
			t.Errorf("Incorrect position: expected column 3, got %d", res.Context.Position.Column)
		}
		if string(res.Context.Remaining) != "c" {
			t.Errorf("Incorrect remaining context: expected \"c\", got %q", string(res.Context.Remaining))
		}

		if res.Result.Left != 'a' {
			t.Errorf("Incorrect result: expected 'a', got %q", res.Result.Left)
		}
		if res.Result.Right != 'b' {
			t.Errorf("Incorrect result: expected 'b', got %q", res.Result.Right)
		}
	})

}

func TestSatisfy(t *testing.T) {
	t.Run("Success: predicate matches", func(t *testing.T) {
		input := "abc"
		ctx := NewParsingContext(input)
		p := Satisfy(func(r rune) bool {
			return r == 'a'
		})

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
	})

	t.Run("Failure: predicate does not match", func(t *testing.T) {
		input := "abc"
		ctx := NewParsingContext(input)
		p := Satisfy(func(r rune) bool {
			return r == 'z'
		})

		_, err := p(ctx)
		if err == nil {
			t.Fatal("An error was expected (predicate mismatch), but nil was returned")
		}

		expectedErr := "'a' is not expected"
		if err.Error() != expectedErr {
			t.Errorf("Incorrect error message: expected %q, got %q", expectedErr, err.Error())
		}
	})

	t.Run("Failure: end of string", func(t *testing.T) {
		input := ""
		ctx := NewParsingContext(input)
		p := Satisfy(func(r rune) bool {
			return true
		})

		_, err := p(ctx)
		if err == nil {
			t.Fatal("An error was expected (end of string), but nil was returned")
		}

		if err.Error() != "end of string" {
			t.Errorf("Incorrect error message: expected \"end of string\", got %q", err.Error())
		}
	})
}

func TestAnyChar(t *testing.T) {
	t.Run("Success: read any character", func(t *testing.T) {
		input := "abc"
		ctx := NewParsingContext(input)
		p := AnyChar()

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
	})

	t.Run("Failure: end of string", func(t *testing.T) {
		input := ""
		ctx := NewParsingContext(input)
		p := AnyChar()

		_, err := p(ctx)
		if err == nil {
			t.Fatal("An error was expected (end of string), but nil was returned")
		}

		if err.Error() != "end of string" {
			t.Errorf("Incorrect error message: expected \"end of string\", got %q", err.Error())
		}
	})
}

func TestMany(t *testing.T) {
	t.Run("Success: match several characters", func(t *testing.T) {
		input := "aaab"
		ctx := NewParsingContext(input)
		p := Many(OneChar('a'))

		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(res.Result) != 3 {
			t.Errorf("Incorrect result length: expected 3, got %d", len(res.Result))
		}

		if string(res.Context.Remaining) != "b" {
			t.Errorf("Incorrect remaining context: expected \"b\", got %q", string(res.Context.Remaining))
		}
	})

	t.Run("Success: match zero characters (0 or more)", func(t *testing.T) {
		input := "bbba"
		ctx := NewParsingContext(input)
		p := Many(OneChar('a'))

		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(res.Result) != 0 {
			t.Errorf("Incorrect result length: expected 0, got %d", len(res.Result))
		}

		if string(res.Context.Remaining) != "bbba" {
			t.Errorf("Incorrect remaining context: expected \"bbba\", got %q", string(res.Context.Remaining))
		}
	})

	t.Run("Success: end of string", func(t *testing.T) {
		input := ""
		ctx := NewParsingContext(input)
		p := Many(OneChar('a'))

		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(res.Result) != 0 {
			t.Errorf("Incorrect result length: expected 0, got %d", len(res.Result))
		}
	})
}
