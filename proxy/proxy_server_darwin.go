package proxy

import (
	"httpStackLens/proxy/middlewares"
	"net/url"
)

func ConfigureOsSpecificProxyPipeline(outputProxy *url.URL) (middlewares.Middleware, error) {
	return ConfigureProxyPipelineBase(outputProxy), nil
}
