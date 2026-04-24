package main

import (
	"flag"
	"fmt"
	configuration "httpStackLens/configuration"
	"httpStackLens/proxy"
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

	pipeline, err := proxy.ConfigureOsSpecificProxyPipeline(*outputProxy, useOutputProxy, requireWindowsAuthentication, addWindowsAuthenticationToOutputProxy)
	if err != nil {
		return AppContext{}, err
	}

	return AppContext{
		pipeline:  pipeline,
		port:      port,
		webUiPort: webUiPort,
	}, nil

}
