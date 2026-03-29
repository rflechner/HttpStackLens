package http

import (
	"bufio"
	"container/list"
	"errors"
	"fmt"
	"httpStackLens/http/models"
	"httpStackLens/http/parser"
	"io"
	"strings"

	p "github.com/rflechner/EasyParsingForGo/combinator"
)

func ReadProxyRequest(reader io.Reader) (models.ProxyRequest, error) {
	scanner := bufio.NewScanner(reader)

	connect, err := readConnect(scanner)
	if err != nil {
		return models.ProxyRequest{}, err
	}

	headers := list.New()

	for scanner.Scan() {
		message := strings.TrimSpace(scanner.Text())
		if len(message) == 0 {
			break
		}
		context := p.NewParsingContext(message)
		result, err := parser.HeaderParser()(context)
		if err != nil {
			fmt.Printf("Error parsing message: %v in '%s'\n", err, message)
			break
		}

		headers.PushBack(result.Result)
	}

	if err := scanner.Err(); err != nil {
		return models.ProxyRequest{}, err
	}

	return models.ProxyRequest{HttpRequestLine: connect, Headers: ListToSlice[models.Header](headers)}, nil
}

func ReadHttpResponse(reader io.Reader) (models.HttpResponseHead, error) {
	scanner := bufio.NewScanner(reader)

	if !scanner.Scan() {
		return models.HttpResponseHead{}, errors.New("connection closed before response status")
	}

	statusLine := strings.TrimSpace(scanner.Text())
	context := p.NewParsingContext(statusLine)
	result, err := parser.ResponseHeadParser()(context)
	if err != nil {
		return models.HttpResponseHead{}, fmt.Errorf("error parsing response status: %w in '%s'", err, statusLine)
	}

	head := result.Result
	headers := list.New()

	for scanner.Scan() {
		message := strings.TrimSpace(scanner.Text())
		if len(message) == 0 {
			break
		}
		context := p.NewParsingContext(message)
		hResult, err := parser.HeaderParser()(context)
		if err != nil {
			fmt.Printf("Error parsing header: %v in '%s'\n", err, message)
			break
		}
		headers.PushBack(hResult.Result)
	}

	head.Headers = ListToSlice[models.Header](headers)

	if err := scanner.Err(); err != nil {
		return models.HttpResponseHead{}, err
	}

	return head, nil
}

func readConnect(scanner *bufio.Scanner) (models.HttpRequestLine, error) {
	if scanner.Scan() {
		message := strings.TrimSpace(scanner.Text())
		context := p.NewParsingContext(message)
		result, err := parser.HttpRequestLineParser()(context)
		if err != nil {
			fmt.Printf("Error parsing message: %v in '%s'\n", err, message)
			return models.HttpRequestLine{}, err
		}

		return result.Result, nil
	}
	return models.HttpRequestLine{}, errors.New("Connection seems to be closed")
}
