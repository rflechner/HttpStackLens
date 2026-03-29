package main

import (
	"flag"
	"httpStackLens/proxy"
	"log"
	"net/url"
)

func CreateOsSpecificProxyPipeline() (AppContext, error) {
	port := flag.Int("port", 3128, "listening port")
	outputProxyUri := flag.String("output-proxy-uri", "", "URI to output proxy information")                                                                                                         // -output-proxy-uri=http://localhost:3129/
	requireWindowsAuthentication := flag.Bool("windows-auth-require-ntlm", false, "specifies that browsers need negotiate authentication (Windows supported only)")                                  //-require-negotiate=true
	addWindowsAuthenticationToOutputProxy := flag.Bool("output-proxy-add-windows-auth", false, "specifies that this proxy adds windows authentication to the remote proxy (Windows supported only)") //-output-proxy-add-windows-auth=true
	flag.Parse()

	var outputProxy *url.URL
	if len(*outputProxyUri) > 0 {
		u, err := url.Parse(*outputProxyUri)
		if err != nil {
			log.Printf("Invalid output proxy URI: %v\n", err)
			return AppContext{}, err
		}
		outputProxy = u
	}

	pipeline, err := proxy.ConfigureOsSpecificProxyPipeline(outputProxy, *requireWindowsAuthentication, *addWindowsAuthenticationToOutputProxy)
	if err != nil {
		return AppContext{}, err
	}

	return AppContext{
		pipeline: pipeline,
		port:     *port,
	}, nil

}
