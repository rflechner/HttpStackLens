package proxy

import (
	"httpStackLens/proxy/middlewares"
	"net/url"
)

func ConfigureOsSpecificProxyPipeline(outputProxy url.URL, useOutputProxy bool, noProxy []string) (middlewares.Middleware, error) {
	base := ConfigureProxyPipelineBase(outputProxy, useOutputProxy)
	return WrapNoProxy(base, useOutputProxy, noProxy), nil
}
