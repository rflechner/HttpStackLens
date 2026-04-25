package main

import (
	"fmt"
	"httpStackLens/configuration"
	"httpStackLens/http"
	"httpStackLens/http/models"
	"httpStackLens/proxy/middlewares"
	"log"
	"net"
	"os"
	"sync"
)

type ProxyServer struct {
	listener    net.Listener
	appContext  AppContext
	mu          sync.Mutex
	closed      bool
	EventLogger ProxyEventLogger
}

type ProxyEventLogger interface {
	LogEvent(event string)
	LogRequest(id int, request models.ProxyRequest)
}

func CreateProxyServer(appContext AppContext, eventLogger ProxyEventLogger, config configuration.ProxyConfig) ProxyServer {
	log.Printf("Socket server started on port %v\n", appContext.port)
	var addr string
	if config.EnableRemoteConnection {
		addr = fmt.Sprintf("0.0.0.0:%d", appContext.port)
		fmt.Printf("❗🔓 Proxy accepting remote connections on port %d\n", appContext.port)
	} else {
		addr = fmt.Sprintf("127.0.0.1:%d", appContext.port)
		fmt.Printf("✅🔒 Proxy restricted to localhost on port %d\n", appContext.port)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}

	return ProxyServer{
		listener:    listener,
		appContext:  appContext,
		EventLogger: eventLogger,
	}
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

func (s *ProxyServer) Run() {
	requestId := 0
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
		requestId++
		go s.handleRequest(browser, requestId)(s.appContext.pipeline)
	}
}

func (s *ProxyServer) handleRequest(browser net.Conn, requestId int) func(pipeline middlewares.Middleware) {
	stream := http.NewNetworkStream(browser)
	request, err := http.ReadProxyRequest(stream)
	if err != nil {
		fmt.Printf("Error reading request from %s: %v\n", browser.RemoteAddr().String(), err)
		return func(pipeline middlewares.Middleware) {}
	}

	return func(pipeline middlewares.Middleware) {
		defer func(browser net.Conn) {
			_ = browser.Close()
		}(browser)

		s.EventLogger.LogRequest(requestId, request)

		err := pipeline.HandleProxyRequest(browser, request)
		if err != nil {
			fmt.Printf("Error handling request from %s: %v\n", browser.RemoteAddr().String(), err)
		}
	}
}
