package proxy

import (
	"fmt"
	"net/url"
)

func ConfigureOsSpecificProxyPipeline(outputProxy *url.URL, requireWindowsAuthentication bool) (Middleware, error) {
	if requireWindowsAuthentication {
		return nil, fmt.Errorf("windows authentication is not supported")
	}
	return ConfigureProxyPipelineBase(outputProxy), nil
}
