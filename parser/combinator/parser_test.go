package combinator

import (
	"slices"
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

	t.Run("Success: match one characters", func(t *testing.T) {
		input := "abcd"
		ctx := NewParsingContext(input)
		p := Many(OneChar('a'))

		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(res.Result) != 1 {
			t.Errorf("Incorrect result length: expected 1, got %d", len(res.Result))
		}

		if string(res.Context.Remaining) != "bcd" {
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

func TestOptional(t *testing.T) {
	t.Run("Success: match present", func(t *testing.T) {
		input := "abc"
		ctx := NewParsingContext(input)
		p := Optional(OneChar('a'))

		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if res.Result.IsNone() {
			t.Error("Expected Some, got None")
		}

		if res.Result.Unwrap() != 'a' {
			t.Errorf("Incorrect result: expected 'a', got %q", res.Result.Unwrap())
		}

		if string(res.Context.Remaining) != "bc" {
			t.Errorf("Incorrect remaining context: expected \"bc\", got %q", string(res.Context.Remaining))
		}
	})

	t.Run("Success: match absent", func(t *testing.T) {
		input := "bc"
		ctx := NewParsingContext(input)
		p := Optional(OneChar('a'))

		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if res.Result.IsSome() {
			t.Errorf("Expected None, got Some(%q)", res.Result.Unwrap())
		}

		if string(res.Context.Remaining) != "bc" {
			t.Errorf("Incorrect remaining context: expected \"bc\", got %q", string(res.Context.Remaining))
		}
	})

	t.Run("Success: end of string", func(t *testing.T) {
		input := ""
		ctx := NewParsingContext(input)
		p := Optional(OneChar('a'))

		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if res.Result.IsSome() {
			t.Error("Expected None, got Some")
		}

		if !res.Context.AtEnd() {
			t.Error("Expected context to be at end")
		}
	})
}

func TestOrElse(t *testing.T) {
	t.Run("Success: first parser succeeds", func(t *testing.T) {
		input := "abc"
		ctx := NewParsingContext(input)
		p := OrElse(OneChar('a'), OneChar('b'))

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

	t.Run("Success: second parser succeeds when first fails", func(t *testing.T) {
		input := "bac"
		ctx := NewParsingContext(input)
		p := OrElse(OneChar('a'), OneChar('b'))

		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if res.Result != 'b' {
			t.Errorf("Incorrect result: expected 'b', got %q", res.Result)
		}

		if string(res.Context.Remaining) != "ac" {
			t.Errorf("Incorrect remaining context: expected \"ac\", got %q", string(res.Context.Remaining))
		}
	})

	t.Run("Failure: all parsers fail", func(t *testing.T) {
		input := "cab"
		ctx := NewParsingContext(input)
		p := OrElse(OneChar('a'), OneChar('b'))

		_, err := p(ctx)
		if err == nil {
			t.Fatal("An error was expected (all parsers failed), but nil was returned")
		}

		// OrElse uses errors.Join which produces a multi-line error message usually.
		// Let's just check that it contains the individual errors if possible,
		// or at least that it's not nil.
		t.Logf("Joined error: %v", err)
	})

	t.Run("Failure: empty parsers list", func(t *testing.T) {
		input := "abc"
		ctx := NewParsingContext(input)
		p := OrElse[rune]()

		_, err := p(ctx)
		if err == nil {
			t.Fatal("An error was expected (empty parsers list), but nil was returned")
		}

		expectedErr := "no parsers provided to OrElse"
		if err.Error() != expectedErr {
			t.Errorf("Incorrect error message: expected %q, got %q", expectedErr, err.Error())
		}
	})
}

func TestStringMatch(t *testing.T) {
	t.Run("Success: match full string", func(t *testing.T) {
		input := "hello world"
		ctx := NewParsingContext(input)
		p := StringMatch("hello")

		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if res.Result != "hello" {
			t.Errorf("Incorrect result: expected \"hello\", got %q", res.Result)
		}

		if string(res.Context.Remaining) != " world" {
			t.Errorf("Incorrect remaining context: expected \" world\", got %q", string(res.Context.Remaining))
		}

		if res.Context.Position.Offset != 5 {
			t.Errorf("Incorrect position: expected offset 5, got %d", res.Context.Position.Offset)
		}
	})

	t.Run("Failure: string does not match", func(t *testing.T) {
		input := "hello world"
		ctx := NewParsingContext(input)
		p := StringMatch("world")

		result, err := p(ctx)
		if err == nil {
			t.Fatal("An error was expected (string mismatch), but nil was returned")
		}

		if err.Error() != "string does not match" {
			t.Errorf("Incorrect error message: expected \"string does not match\", got %q", err.Error())
		}

		if string(result.Context.Remaining) != "hello world" {
			t.Errorf("Incorrect remaining context: expected \" world\", got %q", string(result.Context.Remaining))
		}
	})

	t.Run("Failure: end of string", func(t *testing.T) {
		input := ""
		ctx := NewParsingContext(input)
		p := StringMatch("hello")

		_, err := p(ctx)
		if err == nil {
			t.Fatal("An error was expected (end of string), but nil was returned")
		}

		if err.Error() != "end of string" {
			t.Errorf("Incorrect error message: expected \"end of string\", got %q", err.Error())
		}
	})

	t.Run("Failure: input shorter than target string", func(t *testing.T) {
		input := "hel"
		ctx := NewParsingContext(input)
		p := StringMatch("hello")

		_, err := p(ctx)
		if err == nil {
			t.Fatal("An error was expected (input too short), but nil was returned")
		}

		if err.Error() != "string does not match" {
			t.Errorf("Incorrect error message: expected \"string does not match\", got %q", err.Error())
		}
	})
}

func TestUntilText(t *testing.T) {
	t.Run("Success: UntilText with ManySatisfy-like parser", func(t *testing.T) {
		input := "my_json_prop :delimiter: 1234"
		ctx := NewParsingContext(input)

		innerParser := Many(Satisfy(func(r rune) bool {
			return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' || r == '_'
		}))

		p := UntilText(innerParser, ":delimiter:", false)

		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		resultStr := string(res.Result)
		expectedResult := "my_json_prop "
		if resultStr != expectedResult {
			t.Errorf("Incorrect result: expected %q, got %q", expectedResult, resultStr)
		}

		expectedRemaining := ":delimiter: 1234"
		if string(res.Context.Remaining) != expectedRemaining {
			t.Errorf("Incorrect remaining context: expected %q, got %q", expectedRemaining, string(res.Context.Remaining))
		}

		if res.Context.Position.Offset != 13 {
			t.Errorf("Incorrect offset: expected 13, got %d", res.Context.Position.Offset)
		}
		if res.Context.Position.Line != 1 {
			t.Errorf("Incorrect line: expected 1, got %d", res.Context.Position.Line)
		}

		if res.Context.Position.Column != 14 {
			t.Errorf("Incorrect column: expected 14, got %d", res.Context.Position.Column)
		}
	})

	t.Run("Success: UntilText including delimiter", func(t *testing.T) {
		input := "part1:delimiter:part2"
		ctx := NewParsingContext(input)
		innerParser := Many(Satisfy(func(r rune) bool { return r != ':' }))

		p := UntilText(innerParser, ":delimiter:", true)

		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if string(res.Result) != "part1" {
			t.Errorf("Incorrect result: expected \"part1\", got %q", string(res.Result))
		}

		if string(res.Context.Remaining) != "part2" {
			t.Errorf("Incorrect remaining context: expected \"part2\", got %q", string(res.Context.Remaining))
		}

		// "part1" (5) + ":delimiter:" (11) = 16
		if res.Context.Position.Offset != 16 {
			t.Errorf("Incorrect offset: expected 16, got %d", res.Context.Position.Offset)
		}
	})

	t.Run("Failure: delimiter not found", func(t *testing.T) {
		input := "some text without separator"
		ctx := NewParsingContext(input)
		innerParser := Many(AnyChar())

		p := UntilText(innerParser, ":separator:", false)

		_, err := p(ctx)
		if err == nil {
			t.Fatal("Expected error (delimiter not found), got nil")
		}

		if err.Error() != "delimiter not found" {
			t.Errorf("Incorrect error message: expected \"delimiter not found\", got %q", err.Error())
		}
	})

	t.Run("Failure: inner parser did not consume all input", func(t *testing.T) {
		input := "abc:delimiter:def"
		ctx := NewParsingContext(input)

		innerParser := OneChar('a')

		p := UntilText(innerParser, ":delimiter:", false)

		_, err := p(ctx)
		if err == nil {
			t.Fatal("Expected error (did not consume all input), got nil")
		}

	})
}

func TestBetween(t *testing.T) {
	t.Run("Success: parse between parentheses", func(t *testing.T) {
		input := "(a)bc"
		ctx := NewParsingContext(input)
		p := Between(OneChar('('), OneChar('a'), OneChar(')'))

		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if res.Result.Before != '(' {
			t.Errorf("Incorrect Before: expected '(', got %q", res.Result.Before)
		}
		if res.Result.Middle != 'a' {
			t.Errorf("Incorrect Middle: expected 'a', got %q", res.Result.Middle)
		}
		if res.Result.After != ')' {
			t.Errorf("Incorrect After: expected ')', got %q", res.Result.After)
		}

		if string(res.Context.Remaining) != "bc" {
			t.Errorf("Incorrect remaining context: expected \"bc\", got %q", string(res.Context.Remaining))
		}

		if res.Context.Position.Offset != 3 {
			t.Errorf("Incorrect offset: expected 3, got %d", res.Context.Position.Offset)
		}
	})

	t.Run("Failure: before parser fails", func(t *testing.T) {
		input := "[a)bc"
		ctx := NewParsingContext(input)
		p := Between(OneChar('('), OneChar('a'), OneChar(')'))

		_, err := p(ctx)
		if err == nil {
			t.Fatal("Expected error (before mismatch), got nil")
		}
	})

	t.Run("Failure: middle parser fails", func(t *testing.T) {
		input := "(b)bc"
		ctx := NewParsingContext(input)
		p := Between(OneChar('('), OneChar('a'), OneChar(')'))

		_, err := p(ctx)
		if err == nil {
			t.Fatal("Expected error (middle mismatch), got nil")
		}
	})

	t.Run("Failure: after parser fails", func(t *testing.T) {
		input := "(a]bc"
		ctx := NewParsingContext(input)
		p := Between(OneChar('('), OneChar('a'), OneChar(')'))

		_, err := p(ctx)
		if err == nil {
			t.Fatal("Expected error (after mismatch), got nil")
		}
	})
}

func TestSeparatedBy(t *testing.T) {

	t.Run("Success: SeparatedBy with matchTailingSeparator=false", func(t *testing.T) {
		input := "a,b,c"
		ctx := NewParsingContext(input)
		p := SeparatedBy(Satisfy(func(c rune) bool { return c >= 'a' && c <= 'z' }), OneChar(','), false)
		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(res.Result) != 3 {
			t.Errorf("Incorrect result length: expected 3, got %d", len(res.Result))
		}

		if string(res.Context.Remaining) != "" {
			t.Errorf("Incorrect remaining context: expected \"\", got %q", string(res.Context.Remaining))
		}

		if res.Context.Position.Offset != 5 {
			t.Errorf("Incorrect position offset: expected 5, got %d", res.Context.Position.Offset)
		}

		if !slices.Equal(res.Result, []rune{'a', 'b', 'c'}) {
			t.Errorf("Incorrect result: expected [a b c], got %v", res.Result)
		}
	})

	t.Run("Success: SeparatedBy with matchTailingSeparator=false having tailing separator", func(t *testing.T) {
		input := "a,b,c,"
		ctx := NewParsingContext(input)
		p := SeparatedBy(Satisfy(func(c rune) bool { return c >= 'a' && c <= 'z' }), OneChar(','), false)
		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(res.Result) != 3 {
			t.Errorf("Incorrect result length: expected 3, got %d", len(res.Result))
		}

		if string(res.Context.Remaining) != "," {
			t.Errorf("Incorrect remaining context: expected \"\", got %q", string(res.Context.Remaining))
		}

		if res.Context.Position.Offset != 5 {
			t.Errorf("Incorrect position offset: expected 5, got %d", res.Context.Position.Offset)
		}

		if !slices.Equal(res.Result, []rune{'a', 'b', 'c'}) {
			t.Errorf("Incorrect result: expected [a b c], got %v", res.Result)
		}
	})

	t.Run("Success: SeparatedBy with matchTailingSeparator=true", func(t *testing.T) {
		input := "a,b,c"
		ctx := NewParsingContext(input)
		p := SeparatedBy(Satisfy(func(c rune) bool { return c >= 'a' && c <= 'z' }), OneChar(','), true)
		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(res.Result) != 3 {
			t.Errorf("Incorrect result length: expected 3, got %d", len(res.Result))
		}

		if string(res.Context.Remaining) != "" {
			t.Errorf("Incorrect remaining context: expected \"\", got %q", string(res.Context.Remaining))
		}

		if res.Context.Position.Offset != 5 {
			t.Errorf("Incorrect position offset: expected 5, got %d", res.Context.Position.Offset)
		}

		if !slices.Equal(res.Result, []rune{'a', 'b', 'c'}) {
			t.Errorf("Incorrect result: expected [a b c], got %v", res.Result)
		}
	})

	t.Run("Success: SeparatedBy with matchTailingSeparator=true having tailing separator", func(t *testing.T) {
		input := "a,b,c,"
		ctx := NewParsingContext(input)
		p := SeparatedBy(Satisfy(func(c rune) bool { return c >= 'a' && c <= 'z' }), OneChar(','), true)
		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(res.Result) != 3 {
			t.Errorf("Incorrect result length: expected 3, got %d", len(res.Result))
		}

		if string(res.Context.Remaining) != "" {
			t.Errorf("Incorrect remaining context: expected \"\", got %q", string(res.Context.Remaining))
		}

		if res.Context.Position.Offset != 6 {
			t.Errorf("Incorrect position offset: expected 5, got %d", res.Context.Position.Offset)
		}

		if !slices.Equal(res.Result, []rune{'a', 'b', 'c'}) {
			t.Errorf("Incorrect result: expected [a b c], got %v", res.Result)
		}
	})

	t.Run("Failure: SeparatedBy with no items found", func(t *testing.T) {
		input := "1,2,3"
		ctx := NewParsingContext(input)
		// Parser that only matches letters
		p := SeparatedBy(Satisfy(func(c rune) bool { return c >= 'a' && c <= 'z' }), OneChar(','), false)
		_, err := p(ctx)
		if err == nil {
			t.Fatal("Expected error (no items found), got nil")
		}

		expectedErr := "no items found"
		if err.Error() != expectedErr {
			t.Errorf("Incorrect error message: expected %q, got %q", expectedErr, err.Error())
		}
	})

}

func TestSpaces(t *testing.T) {
	t.Run("Success: trim leading spaces", func(t *testing.T) {
		input := "   abc"
		ctx := NewParsingContext(input)
		p := Spaces()

		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if string(res.Result) != "   " {
			t.Errorf("Incorrect result: expected \"   \", got %q", string(res.Result))
		}

		if string(res.Context.Remaining) != "abc" {
			t.Errorf("Incorrect remaining context: expected \"abc\", got %q", string(res.Context.Remaining))
		}
	})

	t.Run("Success: trim mixed whitespace (tabs, newlines, carriage returns)", func(t *testing.T) {
		input := " \t\n\r abc"
		ctx := NewParsingContext(input)
		p := Spaces()

		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if string(res.Result) != " \t\n\r " {
			t.Errorf("Incorrect result length: expected 5 chars, got %d", len(res.Result))
		}

		if string(res.Context.Remaining) != "abc" {
			t.Errorf("Incorrect remaining context: expected \"abc\", got %q", string(res.Context.Remaining))
		}
	})

	t.Run("Success: no whitespace found", func(t *testing.T) {
		input := "abc"
		ctx := NewParsingContext(input)
		p := Spaces()

		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(res.Result) != 0 {
			t.Errorf("Incorrect result length: expected 0, got %d", len(res.Result))
		}

		if string(res.Context.Remaining) != "abc" {
			t.Errorf("Incorrect remaining context: expected \"abc\", got %q", string(res.Context.Remaining))
		}
	})

	t.Run("Success: only whitespace", func(t *testing.T) {
		input := "   "
		ctx := NewParsingContext(input)
		p := Spaces()

		res, err := p(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if string(res.Result) != "   " {
			t.Errorf("Incorrect result: expected \"   \", got %q", string(res.Result))
		}

		if !res.Context.AtEnd() {
			t.Errorf("Expected context to be at end, but remaining is %q", string(res.Context.Remaining))
		}
	})
}

func TestLazyParse(t *testing.T) {
	t.Run("Success: lazy evaluation", func(t *testing.T) {
		input := "abc"
		ctx := NewParsingContext(input)

		// Utilisation de LazyParse pour différer la création du parser
		p := LazyParse(func() Parser[rune] {
			return OneChar('a')
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

	t.Run("Success: mutual recursion", func(t *testing.T) {
		// Exemple simple de récursion : un parser qui matche 'a' suivi optionnellement par lui-même
		// expression := 'a' | 'a' expression
		// Ici on va tester "aaa"

		var parser Parser[string]
		parser = LazyParse(func() Parser[string] {
			return OrElse(
				func(ctx ParsingContext) (ParseResult[string], error) {
					// Cas de base : 'a' suivi de la fin ou d'autre chose que 'a'
					res, err := OneChar('a')(ctx)
					if err != nil {
						return ParseResult[string]{}, err
					}
					// On essaie de continuer la récursion
					nextRes, nextErr := parser(res.Context)
					if nextErr != nil {
						// Si la suite échoue, on retourne juste 'a'
						return ParseResult[string]{
							Result:  "a",
							Context: res.Context,
						}, nil
					}
					return ParseResult[string]{
						Result:  "a" + nextRes.Result,
						Context: nextRes.Context,
					}, nil
				},
				func(ctx ParsingContext) (ParseResult[string], error) {
					res, err := OneChar('a')(ctx)
					if err != nil {
						return ParseResult[string]{}, err
					}
					return ParseResult[string]{
						Result:  "a",
						Context: res.Context,
					}, nil
				},
			)
		})

		input := "aaa"
		ctx := NewParsingContext(input)
		res, err := parser(ctx)

		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if res.Result != "aaa" {
			t.Errorf("Incorrect result: expected \"aaa\", got %q", res.Result)
		}

		if !res.Context.AtEnd() {
			t.Errorf("Expected context to be at end, got %q", string(res.Context.Remaining))
		}
	})

	t.Run("Failure: propagation", func(t *testing.T) {
		input := "bbc"
		ctx := NewParsingContext(input)

		p := LazyParse(func() Parser[rune] {
			return OneChar('a')
		})

		_, err := p(ctx)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
	})
}
