package models

import (
	"fmt"
	"io"
	"strings"
)

type HttpResponseHead struct {
	HttpVersion       Version
	StatusCode        int
	StatusDescription string
	Headers           []Header
}

type HttpBody interface {
	io.WriterTo
	HasContentLength() bool
	ContentLength() int
}

type HttpResponse struct {
	Head HttpResponseHead
	Body HttpBody
}

type BodyString struct {
	Content string
}

func (b BodyString) WriteTo(w io.Writer) (n int64, err error) {
	i, e := w.Write([]byte(b.Content))
	return int64(i), e
}

func (b BodyString) HasContentLength() bool {
	return true
}

func (b BodyString) ContentLength() int {
	return len(b.Content)
}

type EmptyBody struct{}

func (b EmptyBody) WriteTo(_ io.Writer) (int64, error) {
	return 0, nil
}

func (b EmptyBody) HasContentLength() bool {
	return true
}

func (b EmptyBody) ContentLength() int {
	return 0
}

func (h *HttpResponseHead) IsSuccess() bool {
	return h.StatusCode >= 200 && h.StatusCode < 300
}

func (h *HttpResponseHead) IsRedirect() bool {
	return h.StatusCode >= 300 && h.StatusCode < 400
}

func (h *HttpResponseHead) IsClientError() bool {
	return h.StatusCode >= 400 && h.StatusCode < 500
}

func (h *HttpResponseHead) IsServerError() bool {
	return h.StatusCode >= 500 && h.StatusCode < 600
}

func (h *HttpResponseHead) AddHeader(name string, value string) {
	h.Headers = append(h.Headers, Header{Name: name, Value: value})
}

func (h *HttpResponseHead) SetContentLength(n int) {
	val := fmt.Sprintf("%d", n)
	for i := range h.Headers {
		if strings.EqualFold(h.Headers[i].Name, "Content-Length") {
			h.Headers[i].Value = val
			return
		}
	}
	h.AddHeader("Content-Length", val)
}

func (h *HttpResponseHead) String() string {
	lines := []string{fmt.Sprintf("HTTP/%d.%d %d %s", h.HttpVersion.Major, h.HttpVersion.Minor, h.StatusCode, h.StatusDescription)}

	for _, header := range h.Headers {
		lines = append(lines, fmt.Sprintf("%s: %s", header.Name, header.Value))
	}
	lines = append(lines, "")
	return strings.Join(lines, "\r\n") + "\r\n"
}

func (h *HttpResponseHead) GetHeader(name string) []string {
	var values []string
	for _, header := range h.Headers {
		if strings.EqualFold(header.Name, name) {
			values = append(values, header.Value)
		}
	}
	return values
}

func (h *HttpResponseHead) Bytes() []byte {
	return []byte(h.String())
}

func (h *HttpResponseHead) WriteTo(w io.Writer) (int64, error) {
	i, err := w.Write(h.Bytes())
	return int64(i), err
}

func (r *HttpResponse) WriteTo(w io.Writer) (int64, error) {
	if r.Body.HasContentLength() {
		contentLength := r.Body.ContentLength()
		r.Head.SetContentLength(contentLength)
	}
	i, err := r.Head.WriteTo(w)
	if err != nil {
		return i, err
	}
	j, err := r.Body.WriteTo(w)
	if err != nil {
		return j, err
	}
	return i + j, nil
}
