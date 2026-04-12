package main

import (
	"flag"
	configuration "httpStackLens/config"
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

	outputProxyUri := flag.String("output-proxy-uri", "", "URI to output proxy information")                                                                                                         // -output-proxy-uri=http://localhost:3129/
	requireWindowsAuthentication := flag.Bool("windows-auth-require-ntlm", false, "specifies that browsers need negotiate authentication (Windows supported only)")                                  //-require-negotiate=true
	addWindowsAuthenticationToOutputProxy := flag.Bool("output-proxy-add-windows-auth", false, "specifies that this proxy adds windows authentication to the remote proxy (Windows supported only)") //-output-proxy-add-windows-auth=true
	flag.Parse()

	var outputProxy *url.URL
	useOutputProxy := false
	if len(*outputProxyUri) > 0 {
		u, err := url.Parse(*outputProxyUri)
		if err != nil {
			log.Printf("Invalid output proxy URI: %v\n", err)
			return AppContext{}, err
		}
		outputProxy = u
		useOutputProxy = true
	} else {
		outputProxy = &(url.URL{})
	}

	pipeline, err := proxy.ConfigureOsSpecificProxyPipeline(*outputProxy, useOutputProxy, *requireWindowsAuthentication, *addWindowsAuthenticationToOutputProxy)
	if err != nil {
		return AppContext{}, err
	}

	return AppContext{
		pipeline: pipeline,
		port:     port,
	}, nil

}
