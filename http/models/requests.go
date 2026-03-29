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
