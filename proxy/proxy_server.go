package proxy

import (
	"httpStackLens/http/ast"
	"httpStackLens/proxy/middlewares"
	"net"
	"net/url"
)

func ConfigureProxyPipelineBase(outputProxy *url.URL) Middleware {
	if outputProxy != nil {
		return &middlewares.ForwardProxyServer{OutputProxy: *outputProxy}
	}
	return &middlewares.TunnelServer{}
}

type Middleware interface {
	HandleProxyRequest(browser net.Conn, request ast.ProxyRequest) error
}
