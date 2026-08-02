package helpers

import (
	"fmt"
	"net/url"
	"strings"

	p "github.com/rflechner/EasyParsingForGo/combinator"
)

func SpacesParser() p.Parser[struct{}] {
	return p.Skip(p.Spaces())
}
func NewLineParser() p.Parser[string] {
	return p.OrElse(
		p.StringMatch("\r\n"),
		p.StringMatch("\n"))
}

func UrlParser() p.Parser[url.URL] {
	return func(context p.ParsingContext) (p.ParseResult[url.URL], error) {
		text, err := p.UntilText(p.Many(p.Satisfy(func(r rune) bool { return true })), " HTTP/", false)(context)
		if err != nil {
			return p.ParseResult[url.URL]{Context: context}, err
		}

		urlString := string(text.Result)
		if strings.Contains(urlString, "://") == false {
			return p.ParseResult[url.URL]{Context: context}, fmt.Errorf("URL must contain protocol")
		}
		parsedUrl, err := url.Parse(urlString)
		if err != nil {
			return p.ParseResult[url.URL]{Context: context}, err
		}

		return p.ParseResult[url.URL]{
			Result:  *parsedUrl,
			Context: text.Context,
		}, nil
	}
}
