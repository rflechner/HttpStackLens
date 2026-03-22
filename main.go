package main

import (
	"flag"
	"fmt"
	"httpStackLens/http"
	"httpStackLens/proxy"
	"log"
	"net"
	"net/url"
	"os"
)

func main() {
	port := flag.Int("port", 3128, "listening port")
	outputProxyUri := flag.String("output-proxy-uri", "", "URI to output proxy information")                                                                // -output-proxy-uri=http://localhost:3129/
	requireWindowsAuthentication := flag.Bool("require-negotiate", false, "specifies that browsers need negotiate authentication (Windows supported only)") //-require-negotiate=true
	flag.Parse()

	var outputProxy *url.URL
	if len(*outputProxyUri) > 0 {
		u, err := url.Parse(*outputProxyUri)
		if err != nil {
			log.Printf("Invalid output proxy URI: %v\n", err)
			return
		}
		outputProxy = u
	}

	pipeline, err := proxy.ConfigureOsSpecificProxyPipeline(outputProxy, *requireWindowsAuthentication)
	if err != nil {
		log.Printf("Failed to configure proxy pipeline: %v\n", err)
		return
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}
	defer func(listener net.Listener) {
		err = listener.Close()
		if err != nil {
			log.Printf("Warning when closing browser connection: %v\n", err.Error())
		}
	}(listener)

	log.Printf("Socket server started on port %v\n", *port)

	for {
		browser, err := listener.Accept()
		if err != nil {
			log.Println("Error accepting connection:", err)
			continue
		}

		request, err := http.ReadProxyRequest(browser)
		if err != nil {
			fmt.Printf("Error reading request from %s: %v\n", browser.RemoteAddr().String(), err)
			continue
		}
		go func() {
			err := pipeline.HandleProxyRequest(browser, request)
			if err != nil {
				fmt.Printf("Error handling request from %s: %v\n", browser.RemoteAddr().String(), err)
			}
		}()
	}
}
