package proxy

import (
	"httpStackLens/proxy/middlewares"
	"net/url"
)

func ConfigureOsSpecificProxyPipeline(outputProxy url.URL, useOutputProxy bool, requireWindowsAuthentication bool, addWindowsAuthenticationToOutputProxy bool) (middlewares.Middleware, error) {
	basePipeline := ConfigureProxyPipelineBase(outputProxy, useOutputProxy)
	if addWindowsAuthenticationToOutputProxy {
		basePipeline = &middlewares.ForwardProxyServerWithWindowsAuthentication{
			Forwarder: middlewares.ForwardProxyServer{OutputProxy: outputProxy},
		}
	}

	if requireWindowsAuthentication {
		return &middlewares.WindowsAuthenticationServerMiddleware{
			NextMiddleware: basePipeline,
		}, nil
	}
	return basePipeline, nil
}
