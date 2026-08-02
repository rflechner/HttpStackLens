package composer

import (
	"httpStackLens/http/models"
)

type HttpRequestFileItem struct {
	HeaderComments  []string
	HttpRequestLine models.HttpRequestLine
	Headers         []models.Header
	Body            []byte
}

type HttpFile struct {
	Requests []HttpRequestFileItem
}
