package proxy

import (
	"fmt"
	"httpStackLens/proxy/middlewares"
	"net/url"
)

func ConfigureOsSpecificProxyPipeline(outputProxy *url.URL, requireWindowsAuthentication bool) (middlewares.Middleware, error) {
	if requireWindowsAuthentication {
		return nil, fmt.Errorf("windows authentication is not supported")
	}
	return ConfigureProxyPipelineBase(outputProxy), nil
}
