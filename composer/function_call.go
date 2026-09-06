package composer

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	p "github.com/rflechner/EasyParsingForGo/combinator"
)

// FunctionCall describes syntax only: parsing never invokes a handler.
// Parameters are named strings, with single- or double-quoted values decoded using Go
// string escapes. Bare values such as clientSecret: demo-secret are also strings.
type FunctionCall struct {
	Name       string
	Parameters map[string]string
	// tokens retain source spans for the shared syntax highlighter.
	tokens []Token
}

// FunctionHandler is provided by the caller when it explicitly evaluates a
// variable. It decides which functions and parameter names are supported.
// No builtins, network requests or environment lookups are registered here.
type FunctionHandler func(ctx context.Context, name string, parameters map[string]string) (string, error)

// Evaluate returns a literal unchanged or dispatches a parsed call. It does not
// interpolate placeholders or cache results. Handler errors are propagated;
// handlers should avoid including secrets in their error messages.
func (v FileVariable) Evaluate(ctx context.Context, handler FunctionHandler) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if v.Call == nil {
		return v.Value.Text, nil
	}
	if handler == nil {
		return "", fmt.Errorf("function handler is required")
	}
	// A handler can modify its dictionary without changing the parsed file.
	parameters := make(map[string]string, len(v.Call.Parameters))
	for name, value := range v.Call.Parameters {
		parameters[name] = value
	}
	return handler(ctx, v.Call.Name, parameters)
}

func identifierRune(c rune, first bool) bool {
	return unicode.IsLetter(c) || c == '_' || (!first && (unicode.IsDigit(c) || c == '-'))
}

// functionSpaces deliberately uses the same ASCII whitespace as the file grammar.
func functionSpaces() p.Parser[[]rune] {
	return p.Many(p.Satisfy(func(c rune) bool { return strings.ContainsRune(" \t\r\n", c) }))
}

func functionIdentifierParser() p.Parser[string] {
	return p.Map(p.Combine(
		p.Satisfy(func(c rune) bool { return identifierRune(c, true) }),
		p.Many(p.Satisfy(func(c rune) bool { return identifierRune(c, false) })),
	), func(pair struct {
		Left  rune
		Right []rune
	}) string {
		return string(pair.Left) + string(pair.Right)
	})
}

func functionOpeningParser() p.Parser[string] {
	horizontalSpace := p.Many(p.Satisfy(func(c rune) bool { return c == ' ' || c == '\t' }))
	return p.Left(
		functionExpected(functionIdentifierParser(), "expected function name"),
		p.Right(horizontalSpace, functionExpected(p.OneChar('('), "expected opening parenthesis")),
	)
}

func isFunctionCall(context p.ParsingContext) bool {
	_, err := functionOpeningParser()(context)
	return err == nil
}

func functionError(context p.ParsingContext, message string) error {
	return fmt.Errorf("%s at line %d, column %d", message, context.Position.Line, context.Position.Column)
}

// Primitive combinator errors can quote input. Replace them with a diagnostic
// that names the expected syntax without exposing parameter values.
func functionExpected[T any](parser p.Parser[T], message string) p.Parser[T] {
	return func(context p.ParsingContext) (p.ParseResult[T], error) {
		result, err := parser(context)
		if err != nil {
			return p.ParseResult[T]{Context: context}, functionError(context, message)
		}
		return result, nil
	}
}

func functionQuotedValueParser(quote rune) p.Parser[string] {
	plain := p.Map(p.Satisfy(func(c rune) bool {
		return c != quote && c != '\\' && c != '\r' && c != '\n'
	}), func(c rune) string { return string(c) })
	escaped := p.Map(p.Combine(p.OneChar('\\'), p.Satisfy(func(c rune) bool {
		return c != '\r' && c != '\n'
	})), func(pair struct {
		Left  rune
		Right rune
	}) string {
		return string([]rune{pair.Left, pair.Right})
	})
	content := p.Map(p.Many(p.OrElse(escaped, plain)), func(parts []string) string {
		return strings.Join(parts, "")
	})
	quoted := p.Right(p.OneChar(quote), p.Left(content,
		functionExpected(p.OneChar(quote), "unterminated quoted value")))
	return func(context p.ParsingContext) (p.ParseResult[string], error) {
		result, err := quoted(context)
		if err != nil {
			return p.ParseResult[string]{Context: context}, err
		}
		var value strings.Builder
		for remaining := result.Result; remaining != ""; {
			character, multibyte, tail, err := strconv.UnquoteChar(remaining, byte(quote))
			if err != nil {
				return p.ParseResult[string]{Context: context}, functionError(result.Context, "invalid string escape")
			}
			if multibyte {
				value.WriteRune(character)
			} else {
				value.WriteByte(byte(character))
			}
			remaining = tail
		}
		return p.ParseResult[string]{Result: value.String(), Context: result.Context}, nil
	}
}

func functionValueParser() p.Parser[string] {
	bare := p.Map(p.Many(p.Satisfy(func(c rune) bool {
		return !strings.ContainsRune(",) \t\r\n", c)
	})), func(value []rune) string { return string(value) })
	doubleQuoted := functionQuotedValueParser('"')
	singleQuoted := functionQuotedValueParser('\'')
	return func(context p.ParsingContext) (p.ParseResult[string], error) {
		// Commit to a quoted value: an invalid escape must never fall back to bare text.
		if startsWith(context, `"`) {
			return doubleQuoted(context)
		}
		if startsWith(context, "'") {
			return singleQuoted(context)
		}
		result, err := bare(context)
		if err != nil {
			return result, err
		}
		if result.Result == "" {
			return result, functionError(context, "expected parameter value")
		}
		if offset := strings.IndexAny(result.Result, "(\"'"); offset >= 0 {
			position := context.Forward(len([]rune(result.Result[:offset])))
			return result, functionError(position, "expected a string value; nested calls are not supported")
		}
		return result, nil
	}
}

func functionValueSpanParser() p.Parser[PositionedText[string]] {
	parser := functionValueParser()
	return func(context p.ParsingContext) (p.ParseResult[PositionedText[string]], error) {
		result, err := parser(context)
		if err != nil {
			return p.ParseResult[PositionedText[string]]{Context: context}, err
		}
		return p.ParseResult[PositionedText[string]]{
			Result: positioned(context, result.Context, "", result.Result), Context: result.Context,
		}, nil
	}
}

// FunctionCallParser reads name(key: "value", other: bare-value), allowing
// whitespace and line breaks between arguments and an optional trailing comma.
// Positional arguments, nested calls and duplicate parameter names are rejected.
// Returned offsets count runes, just like the rest of the .http parser.
func FunctionCallParser() p.Parser[FunctionCall] {
	opening := functionOpeningParser()
	nameParser := functionExpected(functionIdentifierParser(), "expected named parameter")
	valueParser := p.Right(functionSpaces(), p.Right(
		functionExpected(p.OneChar(':'), "expected colon after parameter name"),
		p.Right(functionSpaces(), functionValueSpanParser()),
	))
	delimiter := functionExpected(p.OrElse(p.OneChar(','), p.OneChar(')')), "expected comma or closing parenthesis")
	return func(context p.ParsingContext) (p.ParseResult[FunctionCall], error) {
		fail := func(err error) (p.ParseResult[FunctionCall], error) {
			return p.ParseResult[FunctionCall]{Context: context}, err
		}
		name, err := opening(context)
		if err != nil {
			return fail(err)
		}
		call := FunctionCall{Name: name.Result, Parameters: make(map[string]string)}
		call.tokens = append(call.tokens, Token{TokenFunction, context.Position.Offset, context.Position.Offset + len([]rune(name.Result))})
		next := name.Context
		// SeparatedBy stops on an invalid argument. Keep this loop explicit so
		// malformed arguments retain their diagnostics and duplicates are rejected.
		for {
			space, _ := functionSpaces()(next)
			next = space.Context
			if next.AtEnd() {
				return fail(functionError(next, "unterminated function call"))
			}
			if closing, err := p.OneChar(')')(next); err == nil {
				return p.ParseResult[FunctionCall]{Result: call, Context: closing.Context}, nil
			}
			key, err := nameParser(next)
			if err != nil {
				return fail(err)
			}
			if _, exists := call.Parameters[key.Result]; exists {
				return fail(functionError(key.Context, "duplicate parameter"))
			}
			value, err := valueParser(key.Context)
			if err != nil {
				return fail(err)
			}
			call.Parameters[key.Result] = value.Result.Text
			call.tokens = append(call.tokens, Token{TokenParameter, next.Position.Offset, key.Context.Position.Offset})
			call.tokens = appendSpan(call.tokens, TokenString, value.Result)
			space, _ = functionSpaces()(value.Context)
			if space.Context.AtEnd() {
				return fail(functionError(space.Context, "unterminated function call"))
			}
			end, err := delimiter(space.Context)
			if err != nil {
				return fail(err)
			}
			if end.Result == ')' {
				return p.ParseResult[FunctionCall]{Result: call, Context: end.Context}, nil
			}
			next = end.Context
		}
	}
}
