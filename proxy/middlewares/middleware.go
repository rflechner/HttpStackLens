package middlewares

import (
	"httpStackLens/http/models"
	"net"
)

type Middleware interface {
	HandleProxyRequest(browser net.Conn, request models.ProxyRequest) error
}
