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

	return models.ProxyRequest{Connect: connect, Headers: ListToSlice[models.Header](headers)}, nil
}

func readConnect(scanner *bufio.Scanner) (models.Connect, error) {
	if scanner.Scan() {
		message := strings.TrimSpace(scanner.Text())
		context := p.NewParsingContext(message)
		result, err := parser.ConnectParser()(context)
		if err != nil {
			fmt.Printf("Error parsing message: %v in '%s'\n", err, message)
			return models.Connect{}, err
		}

		return result.Result, nil
	}
	return models.Connect{}, errors.New("Connection seems to be closed")
}
