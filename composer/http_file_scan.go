package composer

import (
	p "github.com/rflechner/EasyParsingForGo/combinator"
)

// ParseHttpFile reads a whole `.http` file: the `@name = value` variables at
// the top level, then one request per "###" block.
//
// It never fails. A block the grammar cannot read is recorded in Issues and
// skipped up to the next separator, so that a syntax error on line 3 does not
// cost the reader the twelve requests below it — which is what a file being
// typed into looks like most of the time.
func ParseHttpFile(source string) HttpFile {
	file := HttpFile{}
	context := p.NewParsingContext(source)

	for {
		context = skipBlankLines(context)
		if context.AtEnd() {
			return file
		}

		if variable, err := FileVariableParser()(context); err == nil {
			file.Variables = append(file.Variables, variable.Result)
			context = consumeEndOfLine(variable.Context)
			continue
		}

		item, err := HttpRequestFileItemParser()(context)
		// The offset check is the loop's guarantee of progress: a parser that
		// succeeded without consuming anything would spin here forever.
		if err == nil && item.Context.Position.Offset > context.Position.Offset {
			file.Requests = append(file.Requests, item.Result)
			context = item.Context
			continue
		}

		message := "unreadable request block"
		if err != nil {
			message = err.Error()
		}
		var issue ParseIssue
		issue, context = skipUnreadableBlock(context, message)
		file.Issues = append(file.Issues, issue)
	}
}

// skipUnreadableBlock consumes the stretch the block parser choked on, from the
// current line up to the separator that opens the next block, and reports it as
// one issue.
func skipUnreadableBlock(context p.ParsingContext, message string) (ParseIssue, p.ParsingContext) {
	_, next := readLine(context)
	for !next.AtEnd() && !startsWith(next, separator) {
		_, next = readLine(next)
	}

	// The blank lines trailing the block were consumed but are not part of what
	// could not be read.
	start, end, _ := spanOf(context, next, cutSpace+"\n")
	return ParseIssue{Message: message, Start: start, End: end}, next
}
