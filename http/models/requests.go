package models

import (
	"fmt"
	"io"
	"strings"
)

type Command interface {
	isCommand()
}

type HttpMethod string

const (
	GET     HttpMethod = "GET"
	POST    HttpMethod = "POST"
	PUT     HttpMethod = "PUT"
	PATCH   HttpMethod = "PATCH"
	HEAD    HttpMethod = "HEAD"
	OPTIONS HttpMethod = "OPTIONS"
	DELETE  HttpMethod = "DELETE"
	CONNECT HttpMethod = "CONNECT"
)

func ParseHttpMethod(input string) (HttpMethod, error) {
	if strings.EqualFold(string(GET), input) {
		return GET, nil
	}
	if strings.EqualFold(string(POST), input) {
		return POST, nil
	}
	if strings.EqualFold(string(PUT), input) {
		return PUT, nil
	}
	if strings.EqualFold(string(PATCH), input) {
		return PATCH, nil
	}
	if strings.EqualFold(string(HEAD), input) {
		return HEAD, nil
	}
	if strings.EqualFold(string(OPTIONS), input) {
		return OPTIONS, nil
	}
	if strings.EqualFold(string(DELETE), input) {
		return DELETE, nil
	}
	if strings.EqualFold(string(CONNECT), input) {
		return CONNECT, nil
	}
	return "", fmt.Errorf("unknown http method: %s", input)
}

type HttpRequestLine struct {
	HttpMethod HttpMethod
	HostPort   HostPort
	Version    Version
}

type ProxyRequest struct {
	HttpRequestLine HttpRequestLine
	Headers         []Header
}

func (r *ProxyRequest) GetHeader(name string) []string {
	var values []string
	for _, header := range r.Headers {
		if strings.EqualFold(header.Name, name) {
			values = append(values, header.Value)
		}
	}
	return values
}

func (r *ProxyRequest) AddHeader(name, value string) {
	r.Headers = append(r.Headers, Header{Name: name, Value: value})
}

func (r *ProxyRequest) WriteTo(w io.Writer, writeProxyHeader bool) (int64, error) {
	var total int64

	n, err := fmt.Fprintf(w, "%s %s:%d HTTP/%d.%d\r\n",
		r.HttpRequestLine.HttpMethod,
		r.HttpRequestLine.HostPort.Host, r.HttpRequestLine.HostPort.Port,
		r.HttpRequestLine.Version.Major, r.HttpRequestLine.Version.Minor)
	total += int64(n)
	if err != nil {
		return total, err
	}

	for _, header := range r.Headers {
		if !writeProxyHeader && strings.HasPrefix(strings.ToLower(header.Name), "proxy-") {
			continue
		}
		n, err := fmt.Fprintf(w, "%s: %s\r\n", header.Name, header.Value)
		total += int64(n)
		if err != nil {
			return total, err
		}
	}

	n, err = io.WriteString(w, "\r\n")
	total += int64(n)
	return total, err
}
