package proxy

import (
	"httpStackLens/proxy/middlewares"
	"net/url"
)

func ConfigureOsSpecificProxyPipeline(outputProxy url.URL, useOutputProxy bool, noProxy []string, requireWindowsAuthentication bool, addWindowsAuthenticationToOutputProxy bool, treat401AsProxyAuthentication bool) (middlewares.Middleware, error) {
	basePipeline := ConfigureProxyPipelineBase(outputProxy, useOutputProxy)
	if addWindowsAuthenticationToOutputProxy {
		basePipeline = &middlewares.ForwardProxyServerWithWindowsAuthentication{
			Forwarder:                     middlewares.ForwardProxyServer{OutputProxy: outputProxy},
			Treat401AsProxyAuthentication: treat401AsProxyAuthentication,
		}
	}

	// Route no_proxy hosts directly instead of through the upstream (incl. the
	// Windows-auth forwarder above), before the browser-auth wrapper so the browser
	// is still authenticated to us for every request.
	basePipeline = WrapNoProxy(basePipeline, useOutputProxy, noProxy)

	if requireWindowsAuthentication {
		return &middlewares.WindowsAuthenticationServerMiddleware{
			NextMiddleware: basePipeline,
		}, nil
	}
	return basePipeline, nil
}
