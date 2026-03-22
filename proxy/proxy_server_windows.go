package proxy

import (
	"httpStackLens/proxy/middlewares"
	"net/url"
)

func ConfigureOsSpecificProxyPipeline(outputProxy *url.URL, requireWindowsAuthentication bool) (middlewares.Middleware, error) {
	basePipeline := ConfigureProxyPipelineBase(outputProxy)

	if requireWindowsAuthentication {
		return &middlewares.WindowsAuthenticationServerMiddleware{
			NextMiddleware: basePipeline,
		}, nil
	}
	return basePipeline, nil
}
