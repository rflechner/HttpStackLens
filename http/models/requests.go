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
	Endpoint   ResourceEndpoint
	Version    Version
}

func (r *HttpRequestLine) IsConnect() bool {
	return strings.EqualFold(string(r.HttpMethod), string(CONNECT))
}

func (r *HttpRequestLine) String() string {
	if r.IsConnect() {
		return fmt.Sprintf("🔐 %s %s:%d HTTP/%d.%d",
			r.HttpMethod,
			r.Endpoint.Host,
			r.Endpoint.Port,
			r.Version.Major,
			r.Version.Minor)
	}
	return fmt.Sprintf("👀 %s %s%s HTTP/%d.%d",
		r.HttpMethod,
		r.Endpoint.Host,
		r.Endpoint.PathAndQuery,
		r.Version.Major,
		r.Version.Minor)
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

func (r *ProxyRequest) WriteTo(w io.Writer, writeProxyHeader bool) (int, error) {
	var total int
	var err error

	if r.HttpRequestLine.IsConnect() {
		total, err = fmt.Fprintf(w, "%s %s:%d HTTP/%d.%d\r\n",
			r.HttpRequestLine.HttpMethod,
			r.HttpRequestLine.Endpoint.Host, r.HttpRequestLine.Endpoint.Port,
			r.HttpRequestLine.Version.Major, r.HttpRequestLine.Version.Minor)
	} else {
		total, err = fmt.Fprintf(w, "%s %s HTTP/%d.%d\r\n",
			r.HttpRequestLine.HttpMethod,
			r.HttpRequestLine.Endpoint.PathAndQuery,
			r.HttpRequestLine.Version.Major, r.HttpRequestLine.Version.Minor)
	}

	if err != nil {
		return total, err
	}

	for _, header := range r.Headers {
		if !writeProxyHeader && strings.HasPrefix(strings.ToLower(header.Name), "proxy-") {
			continue
		}
		n, err := fmt.Fprintf(w, "%s: %s\r\n", header.Name, header.Value)
		total += n
		if err != nil {
			return total, err
		}
	}

	n, err := io.WriteString(w, "\r\n")
	total += n
	return total, err
}
