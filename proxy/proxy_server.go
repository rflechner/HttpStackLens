package proxy

import (
	"httpStackLens/proxy/middlewares"
	"net/url"
)

func ConfigureProxyPipelineBase(outputProxy *url.URL) middlewares.Middleware {
	if outputProxy != nil {
		return &middlewares.ForwardProxyServer{OutputProxy: *outputProxy}
	}
	return &middlewares.TunnelServer{}
}
