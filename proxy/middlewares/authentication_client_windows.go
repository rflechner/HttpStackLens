package middlewares

import (
	"fmt"
	"httpStackLens/http/ast"
	"net"
)

type AuthenticationClient struct {
}

func (m *AuthenticationClient) HandleProxyRequest(browser net.Conn, request ast.ProxyRequest) error {
	return fmt.Errorf("not implemented")
}
