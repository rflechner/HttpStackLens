package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync"
)

type ProxyServer struct {
	listener   net.Listener
	appContext AppContext
	mu         sync.Mutex
	closed     bool
}

func CreateProxyServer(appContext AppContext) ProxyServer {
	log.Printf("Socket server started on port %v\n", appContext.port)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", appContext.port))
	if err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}

	return ProxyServer{listener: listener, appContext: appContext}
}

func (s *ProxyServer) Close() {
	s.mu.Lock()
	s.closed = true
	err := s.listener.Close()
	if err != nil {
		log.Printf("Warning when closing browser connection: %v\n", err.Error())
	}
	s.mu.Unlock()
}

func (s *ProxyServer) Run(chan bool) {
	for {
		browser, err := s.listener.Accept()
		if err != nil {
			if s.closed {
				return
			}
			log.Println("Error accepting connection:", err)
			continue
		}
		fmt.Printf("New connection from %s\n", browser.RemoteAddr().String())
		go handleRequest(browser)(s.appContext.pipeline)
	}
}
