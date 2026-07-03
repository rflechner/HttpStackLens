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

// WrapNoProxy routes hosts matching the no_proxy list straight to the origin
// instead of forwarding them to the given upstream middleware. It only changes
// behaviour when an upstream proxy is configured — without one, traffic is
// already direct, so it is a no-op (and no_proxy has nothing to bypass).
func WrapNoProxy(upstream middlewares.Middleware, useOutputProxy bool, noProxy []string) middlewares.Middleware {
	if !useOutputProxy || len(noProxy) == 0 {
		return upstream
	}
	return &middlewares.NoProxyRouter{
		Rules:    noProxy,
		Upstream: upstream,
		Direct:   &middlewares.TunnelServer{},
	}
}
