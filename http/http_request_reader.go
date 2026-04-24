package http

import (
	"bufio"
	"container/list"
	"errors"
	"fmt"
	"httpStackLens/http/models"
	"httpStackLens/http/parser"
	"io"
	"net/http/httputil"
	"strconv"
	"strings"

	p "github.com/rflechner/EasyParsingForGo/combinator"
)

func ReadProxyRequest(scanner *bufio.Scanner) (models.ProxyRequest, error) {
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

func ReadHttpResponseBody(reader io.Reader, head models.HttpResponseHead) (models.HttpBody, error) {
	// Check Content-Length
	contentLengthHeaders := head.GetHeader("Content-Length")
	if len(contentLengthHeaders) > 0 {
		contentLength, err := strconv.Atoi(contentLengthHeaders[0])
		if err != nil {
			return nil, fmt.Errorf("invalid Content-Length: %w", err)
		}
		if contentLength == 0 {
			return models.EmptyBody{}, nil
		}
		body := make([]byte, contentLength)
		_, err = io.ReadFull(reader, body)
		if err != nil {
			return nil, fmt.Errorf("failed to read body with Content-Length %d: %w", contentLength, err)
		}
		return models.BodyString{Content: string(body)}, nil
	}

	// Check Transfer-Encoding: chunked
	transferEncodingHeaders := head.GetHeader("Transfer-Encoding")
	isChunked := false
	for _, val := range transferEncodingHeaders {
		if strings.EqualFold(val, "chunked") {
			isChunked = true
			break
		}
	}

	if isChunked {
		// Using net/http/httputil chunked reader
		chunkedReader := httputil.NewChunkedReader(reader)
		body, err := io.ReadAll(chunkedReader)
		if err != nil {
			return nil, fmt.Errorf("failed to read chunked body: %w", err)
		}
		return models.BodyString{Content: string(body)}, nil
	}

	// If no content length and not chunked, for a response, it might be until EOF if it's not a 1xx, 204 or 304
	// But in proxy 407 case, it's likely one of the above.
	return models.EmptyBody{}, nil
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
