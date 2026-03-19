package combinator

type TextPosition struct {
	Offset int
	Line   int
	Column int
}

func (p TextPosition) Forward(past []rune) TextPosition {
	next := p
	for _, c := range past {
		next.Offset++
		if c == '\n' {
			next.Line++
			next.Column = 1
		} else {
			next.Column++
		}
	}
	return next
}

func NewTextPosition() TextPosition {
	return TextPosition{
		Offset: 0,
		Line:   1,
		Column: 1,
	}
}
