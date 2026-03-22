package models

import (
	"fmt"
	"strings"
)

type ResponseHead struct {
	HttpVersion       Version
	StatusCode        int
	StatusDescription string
	Headers           []Header
}

func (h *ResponseHead) IsSuccess() bool {
	return h.StatusCode >= 200 && h.StatusCode < 300
}

func (h *ResponseHead) IsRedirect() bool {
	return h.StatusCode >= 300 && h.StatusCode < 400
}

func (h *ResponseHead) IsClientError() bool {
	return h.StatusCode >= 400 && h.StatusCode < 500
}

func (h *ResponseHead) IsServerError() bool {
	return h.StatusCode >= 500 && h.StatusCode < 600
}

func (h *ResponseHead) AddHeader(name string, value string) {
	h.Headers = append(h.Headers, Header{Name: name, Value: value})
}

func (h *ResponseHead) String() string {
	lines := []string{fmt.Sprintf("HTTP/%d.%d %d %s", h.HttpVersion.Major, h.HttpVersion.Minor, h.StatusCode, h.StatusDescription)}

	for _, header := range h.Headers {
		lines = append(lines, fmt.Sprintf("%s: %s", header.Name, header.Value))
	}
	lines = append(lines, "")
	return strings.Join(lines, "\r\n") + "\r\n"
}

func (h *ResponseHead) GetHeader(name string) string {
	for _, header := range h.Headers {
		if strings.EqualFold(header.Name, name) {
			return header.Value
		}
	}
	return ""
}

func (h *ResponseHead) Bytes() []byte {
	return []byte(h.String())
}
