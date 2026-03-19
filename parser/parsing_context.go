package parser

type ParsingContext struct {
	Remaining []rune
	Position  TextPosition
}

func NewParsingContext(remaining string) ParsingContext {
	return ParsingContext{
		Remaining: []rune(remaining),
		Position: TextPosition{
			Offset: 0,
			Line:   1,
			Column: 1,
		},
	}
}

func (context ParsingContext) AtEnd() bool {
	return len(context.Remaining) == 0
}

func (context ParsingContext) Forward(n int) ParsingContext {
	next := context
	past := context.Remaining[0:n]
	next.Remaining = context.Remaining[n:]
	next.Position = context.Position.Forward(past)
	return next
}
