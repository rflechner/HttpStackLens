package models

import (
	"fmt"
	"io"
	"strings"
)

type Command interface {
	isCommand()
}

type Connect struct {
	HostPort HostPort
	Version  Version
}

type ProxyRequest struct {
	Connect Connect
	Headers []Header
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

func (r *ProxyRequest) WriteTo(w io.Writer, writeProxyHeader bool) (int64, error) {
	var total int64

	n, err := fmt.Fprintf(w, "CONNECT %s:%d HTTP/%d.%d\r\n",
		r.Connect.HostPort.Host, r.Connect.HostPort.Port, r.Connect.Version.Major, r.Connect.Version.Minor)
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
