package proxy

import (
	"httpStackLens/proxy/middlewares"
	"net/url"
)

func ConfigureOsSpecificProxyPipeline(outputProxy url.URL, useOutputProxy bool) (middlewares.Middleware, error) {
	return ConfigureProxyPipelineBase(outputProxy, useOutputProxy), nil
}
