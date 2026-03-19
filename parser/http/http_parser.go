package http

import (
	"fmt"
)
import p "github.com/rflechner/EasyParsingForGo/combinator"

func HTTPVersionParser() p.Parser[HTTPVersion] {
	// HTTP/1.1
	return func(context p.ParsingContext) (p.ParseResult[HTTPVersion], error) {
		res, err := p.StringMatch("HTTP/")(context)
		if err != nil {
			return p.ParseResult[HTTPVersion]{}, err
		}

		majorRes, err := p.Integer()(res.Context)
		if err != nil {
			return p.ParseResult[HTTPVersion]{}, err
		}

		dotRes, err := p.OneChar('.')(majorRes.Context)
		if err != nil {
			return p.ParseResult[HTTPVersion]{}, err
		}

		minorRes, err := p.Integer()(dotRes.Context)
		if err != nil {
			return p.ParseResult[HTTPVersion]{}, err
		}

		return p.ParseResult[HTTPVersion]{
			Result: HTTPVersion{
				Major: majorRes.Result,
				Minor: minorRes.Result,
			},
			Context: minorRes.Context,
		}, nil
	}
}

func ConnectParser() p.Parser[Connect] {
	return func(context p.ParsingContext) (p.ParseResult[Connect], error) {
		// CONNECT
		res, err := p.StringMatch("CONNECT")(context)
		if err != nil {
			return p.ParseResult[Connect]{}, err
		}

		// Space
		spaceRes, err := p.OneChar(' ')(res.Context)
		if err != nil {
			return p.ParseResult[Connect]{}, err
		}

		// Host (until :)
		hostParser := p.Many(p.Satisfy(func(r rune) bool {
			return r != ':' && r != ' '
		}))
		hostRes, err := hostParser(spaceRes.Context)
		if err != nil {
			return p.ParseResult[Connect]{}, err
		}
		if len(hostRes.Result) == 0 {
			return p.ParseResult[Connect]{}, fmt.Errorf("empty host")
		}
		host := string(hostRes.Result)

		// :
		colonRes, err := p.OneChar(':')(hostRes.Context)
		if err != nil {
			return p.ParseResult[Connect]{}, err
		}

		// Port
		portRes, err := p.Integer()(colonRes.Context)
		if err != nil {
			return p.ParseResult[Connect]{}, err
		}

		// Space
		spaceAfterPortRes, err := p.OneChar(' ')(portRes.Context)
		if err != nil {
			return p.ParseResult[Connect]{}, err
		}

		// Version
		versionRes, err := HTTPVersionParser()(spaceAfterPortRes.Context)
		if err != nil {
			return p.ParseResult[Connect]{}, err
		}

		return p.ParseResult[Connect]{
			Result: Connect{
				Host:    host,
				Port:    portRes.Result,
				Version: versionRes.Result,
			},
			Context: versionRes.Context,
		}, nil
	}
}
