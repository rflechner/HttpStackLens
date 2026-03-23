package models

import "strings"

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
