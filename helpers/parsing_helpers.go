package helpers

import (
	"fmt"
	"net/url"
	"strings"

	p "github.com/rflechner/EasyParsingForGo/combinator"
	parsing_helpers "github.com/rflechner/EasyParsingForGo/helpers"
)

func SpacesParser() p.Parser[struct{}] {
	return p.Skip(p.Spaces())
}
func NewLineParser() p.Parser[string] {
	return p.OrElse(
		p.StringMatch("\r\n"),
		p.StringMatch("\n"))
}

// UrlParser reads a request target. It ends on the version marker (" HTTP/")
// when there is one, and otherwise at the end of the line: a `.http` file may
// omit the version, and the caller then applies its own default.
func UrlParser() p.Parser[url.URL] {
	return func(context p.ParsingContext) (p.ParseResult[url.URL], error) {
		length := urlTextLength(context.Remaining)
		if length <= 0 {
			return p.ParseResult[url.URL]{Context: context}, fmt.Errorf("URL is missing")
		}

		urlString := strings.TrimRight(string(context.Remaining[0:length]), " \t\r")
		if strings.Contains(urlString, "://") == false {
			return p.ParseResult[url.URL]{Context: context}, fmt.Errorf("URL must contain protocol")
		}
		parsedUrl, err := url.Parse(urlString)
		if err != nil {
			return p.ParseResult[url.URL]{Context: context}, err
		}

		return p.ParseResult[url.URL]{
			Result:  *parsedUrl,
			Context: context.Forward(length),
		}, nil
	}
}

// urlTextLength is the number of runes making up the target: everything up to
// the version marker, or up to the end of the line when the version is omitted.
func urlTextLength(remaining []rune) int {
	if index := parsing_helpers.IndexOf(remaining, " HTTP/"); index >= 0 {
		return index
	}
	if index := parsing_helpers.IndexOf(remaining, "\n"); index >= 0 {
		return index
	}
	return len(remaining)
}
