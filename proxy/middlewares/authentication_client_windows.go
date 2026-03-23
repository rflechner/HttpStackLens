package middlewares

import (
	"fmt"
	"httpStackLens/http/models"
	"net"
)

type AuthenticationClient struct {
}

func (m *AuthenticationClient) HandleProxyRequest(browser net.Conn, request models.ProxyRequest) error {

	return fmt.Errorf("not implemented")
}
