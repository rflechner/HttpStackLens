package proxy

import (
	"httpStackLens/proxy/middlewares"
	"net/url"
)

func ConfigureProxyPipelineBase(outputProxy url.URL, useOutputProxy bool) middlewares.Middleware {
	if useOutputProxy {
		return &middlewares.ForwardProxyServer{OutputProxy: outputProxy}
	}
	return &middlewares.TunnelServer{}
}
