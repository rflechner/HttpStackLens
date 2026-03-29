package main

import (
	"flag"
	"httpStackLens/proxy"
	"log"
	"net/url"
)

func CreateOsSpecificProxyPipeline() (AppContext, error) {
	port := flag.Int("port", 3128, "listening port")
	outputProxyUri := flag.String("output-proxy-uri", "", "URI to output proxy information")
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

	pipeline, err := proxy.ConfigureOsSpecificProxyPipeline(outputProxy)
	if err != nil {
		return AppContext{}, err
	}

	return AppContext{
		pipeline: pipeline,
		port:     *port,
	}, nil
}
