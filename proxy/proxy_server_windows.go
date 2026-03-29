package proxy

import (
	"httpStackLens/proxy/middlewares"
	"net/url"
)

func ConfigureOsSpecificProxyPipeline(outputProxy *url.URL, requireWindowsAuthentication bool, addWindowsAuthenticationToOutputProxy bool) (middlewares.Middleware, error) {
	basePipeline := ConfigureProxyPipelineBase(outputProxy)
	if addWindowsAuthenticationToOutputProxy {
		basePipeline = &middlewares.ForwardProxyServerWithWindowsAuthentication{
			Forwarder: middlewares.ForwardProxyServer{OutputProxy: *outputProxy},
		}
	}

	if requireWindowsAuthentication {
		return &middlewares.WindowsAuthenticationServerMiddleware{
			NextMiddleware: basePipeline,
		}, nil
	}
	return basePipeline, nil
}
