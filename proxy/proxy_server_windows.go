package proxy

import (
	"httpStackLens/proxy/middlewares"
	"net/url"
)

func ConfigureOsSpecificProxyPipeline(outputProxy url.URL, useOutputProxy bool, requireWindowsAuthentication bool, addWindowsAuthenticationToOutputProxy bool, treat401AsProxyAuthentication bool) (middlewares.Middleware, error) {
	basePipeline := ConfigureProxyPipelineBase(outputProxy, useOutputProxy)
	if addWindowsAuthenticationToOutputProxy {
		basePipeline = &middlewares.ForwardProxyServerWithWindowsAuthentication{
			Forwarder:                     middlewares.ForwardProxyServer{OutputProxy: outputProxy},
			Treat401AsProxyAuthentication: treat401AsProxyAuthentication,
		}
	}

	if requireWindowsAuthentication {
		return &middlewares.WindowsAuthenticationServerMiddleware{
			NextMiddleware: basePipeline,
		}, nil
	}
	return basePipeline, nil
}
