package models

import (
	"fmt"
	"io"
	"net"
	"strconv"
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
	QUERY   HttpMethod = "QUERY"
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
	if strings.EqualFold(string(QUERY), input) {
		return QUERY, nil
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

func (r *ProxyRequest) SetHeader(name, value string) {
	for i := range r.Headers {
		if strings.EqualFold(r.Headers[i].Name, name) {
			r.Headers[i].Value = value
			return
		}
	}
	r.AddHeader(name, value)
}

// WriteTo uses absolute-form and keeps proxy headers when toProxy is true.
// Direct requests use origin-form and omit proxy headers; CONNECT always uses
// authority-form, including when retried during proxy authentication.
func (r *ProxyRequest) WriteTo(w io.Writer, toProxy bool) (int, error) {
	endpoint := r.HttpRequestLine.Endpoint
	target := endpoint.PathAndQuery
	if target == "" || strings.HasPrefix(target, "?") {
		target = "/" + target
	}
	if r.HttpRequestLine.IsConnect() {
		target = net.JoinHostPort(endpoint.Host, strconv.Itoa(endpoint.Port))
	} else if toProxy && target != "*" {
		scheme := endpoint.Scheme
		if scheme == "" {
			scheme = "http"
		}
		host := endpoint.Host
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		defaultPort := scheme == "http" && endpoint.Port == 80 || scheme == "https" && endpoint.Port == 443
		if endpoint.Port != 0 && !defaultPort {
			host = net.JoinHostPort(endpoint.Host, strconv.Itoa(endpoint.Port))
		}
		target = scheme + "://" + host + target
	}
	total, err := fmt.Fprintf(w, "%s %s HTTP/%d.%d\r\n",
		r.HttpRequestLine.HttpMethod, target,
		r.HttpRequestLine.Version.Major, r.HttpRequestLine.Version.Minor)

	if err != nil {
		return total, err
	}

	for _, header := range r.Headers {
		if !toProxy && strings.HasPrefix(strings.ToLower(header.Name), "proxy-") {
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
