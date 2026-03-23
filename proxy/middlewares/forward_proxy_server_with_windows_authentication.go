package middlewares

import (
	"fmt"
	"httpStackLens/http/models"
	"net"
)

type ForwardProxyServerWithWindowsAuthentication struct {
	Forwarder ForwardProxyServer
}

func (m *ForwardProxyServerWithWindowsAuthentication) HandleProxyRequest(browser net.Conn, request models.ProxyRequest) error {
	gateway, err := m.Forwarder.ConnectToGateway(browser, request)
	if err != nil {
		return err
	}
	defer gateway.Close()

	return fmt.Errorf("not implemented")
}
