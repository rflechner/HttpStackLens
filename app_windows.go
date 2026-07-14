package main

import (
	"flag"
	"fmt"
	configuration "httpStackLens/configuration"
	"httpStackLens/proxy"
	"httpStackLens/proxy/middlewares"
	"log"
	"net/url"
)

func CreateOsSpecificProxyPipeline(config configuration.AppConfig) (AppContext, error) {
	port := 3128
	if config.Proxy.Port != 0 {
		port = config.Proxy.Port
	}
	port = *flag.Int("port", port, "listening port")

	webUiPort := 9000
	if config.WebUi.Port != 0 {
		webUiPort = config.WebUi.Port
	}
	webUiPort = *flag.Int("web-ui-port", webUiPort, "listening WEB UI port")

	outputProxyUri := ""
	if config.Proxy.OutputProxyUri != "" {
		outputProxyUri = config.Proxy.OutputProxyUri
	}
	outputProxyUri = *flag.String("output-proxy-uri", outputProxyUri, "URI to output proxy information") // -output-proxy-uri=http://localhost:3129/

	requireWindowsAuthentication := *flag.Bool("windows-auth-require-ntlm", config.Proxy.RequireWindowsAuthentication, "specifies that browsers need negotiate authentication (Windows supported only)") //-require-negotiate=true
	if requireWindowsAuthentication {
		fmt.Println("👮 Enabling Windows authentication for browsers")
	}

	addWindowsAuthenticationToOutputProxy := *flag.Bool("output-proxy-add-windows-auth", config.Proxy.AddWindowsAuthenticationToOutputProxy, "specifies that this proxy adds windows authentication to the remote proxy (Windows supported only)") //-output-proxy-add-windows-auth=true
	if addWindowsAuthenticationToOutputProxy {
		fmt.Println("🙎 Adding Windows authentication to output proxy")
	}

	treat401AsProxyAuthentication := *flag.Bool("output-proxy-treat-401-as-auth-challenge", config.Proxy.Treat401AsProxyAuthentication, "treat upstream 401 WWW-Authenticate responses as proxy authentication challenges (compatibility mode)")
	if treat401AsProxyAuthentication {
		fmt.Println("⚠️ Treating upstream 401 responses as proxy authentication challenges")
	}

	flag.Parse()

	var outputProxy *url.URL
	useOutputProxy := false
	if len(outputProxyUri) > 0 {
		u, err := url.Parse(outputProxyUri)
		if err != nil {
			log.Printf("Invalid output proxy URI: %v\n", err)
			return AppContext{}, err
		}
		outputProxy = u
		useOutputProxy = true
		fmt.Printf("🌍 Using output proxy: %s\n", outputProxyUri)
	} else {
		outputProxy = &(url.URL{})
	}

	pipeline, err := proxy.ConfigureOsSpecificProxyPipeline(*outputProxy, useOutputProxy, config.Proxy.NoProxy, requireWindowsAuthentication, addWindowsAuthenticationToOutputProxy, treat401AsProxyAuthentication)
	if err != nil {
		return AppContext{}, err
	}

	return AppContext{
		pipeline:  pipeline,
		port:      port,
		webUiPort: webUiPort,
	}, nil

}

func RebuildOsSpecificProxyPipeline(config configuration.ProxyConfig) (middlewares.Middleware, error) {
	outputProxy := &url.URL{}
	useOutputProxy := false
	if config.OutputProxyUri != "" {
		parsed, err := url.Parse(config.OutputProxyUri)
		if err != nil {
			return nil, err
		}
		outputProxy = parsed
		useOutputProxy = true
	}
	return proxy.ConfigureOsSpecificProxyPipeline(
		*outputProxy,
		useOutputProxy,
		config.NoProxy,
		config.RequireWindowsAuthentication,
		config.AddWindowsAuthenticationToOutputProxy,
		config.Treat401AsProxyAuthentication,
	)
}
