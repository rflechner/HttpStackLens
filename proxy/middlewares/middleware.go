package middlewares

import (
	"httpStackLens/http/ast"
	"net"
)

type Middleware interface {
	HandleProxyRequest(browser net.Conn, request ast.ProxyRequest) error
}
