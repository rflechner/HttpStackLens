package http

import (
	"bufio"
	"container/list"
	"errors"
	"fmt"
	"goproxy/http/ast"
	"goproxy/http/parser"
	"io"

	p "github.com/rflechner/EasyParsingForGo/combinator"
)

func ReadProxyRequest(reader io.Reader) (ast.ProxyRequest, error) {
	scanner := bufio.NewScanner(reader)

	connect, err := readConnect(scanner)
	if err != nil {
		return ast.ProxyRequest{}, err
	}

	headers := list.New()

	for scanner.Scan() {
		message := scanner.Text()
		context := p.NewParsingContext(message)
		result, err := parser.HeaderParser()(context)
		if err != nil {
			fmt.Printf("Error parsing message: %v\n", err)
			break
		}

		headers.PushBack(result.Result)
		//fmt.Printf("[Command] -> Connect to %s:%d\n", result.Result.HostPort.Host, result.Result.HostPort.Port)
	}

	if err := scanner.Err(); err != nil {
		return ast.ProxyRequest{}, err
	}

	return ast.ProxyRequest{Connect: connect, Headers: ListToSlice[ast.Header](headers)}, nil
}

func readConnect(scanner *bufio.Scanner) (ast.Connect, error) {
	if scanner.Scan() {
		message := scanner.Text()
		context := p.NewParsingContext(message)
		result, err := parser.ConnectParser()(context)
		if err != nil {
			fmt.Printf("Error parsing message: %v\n", err)
			return ast.Connect{}, err
		}

		return result.Result, nil
	}
	return ast.Connect{}, errors.New("failed to read connect message")
}
