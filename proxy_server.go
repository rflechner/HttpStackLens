package main

import (
	"fmt"
	"httpStackLens/configuration"
	"httpStackLens/http"
	"httpStackLens/http/models"
	"httpStackLens/proxy/middlewares"
	"httpStackLens/storage"
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
	// capture, when non-nil, persists top-level requests (HTTP and CONNECT) to
	// the capture file. Decrypted bodies are recorded by the HTTPS interceptor.
	capture storage.CaptureSessionWriter
	// store, when non-nil, keeps recent top-level request records in memory for
	// on-demand inspection by the Web UI.
	store         *storage.RequestStore
	captureCtl    *storage.CaptureController
	accessControl *configuration.AccessControlSettingsStore
}

type ProxyEventLogger interface {
	LogEvent(event string)
	LogRequest(id int, correlationID string, request models.ProxyRequest)
}

type pipelineSnapshotter interface {
	Snapshot() (middlewares.Middleware, bool)
}

func CreateProxyServer(appContext AppContext, eventLogger ProxyEventLogger, config configuration.ProxyConfig, accessControl *configuration.AccessControlSettingsStore, capture storage.CaptureSessionWriter, store *storage.RequestStore, captureCtl *storage.CaptureController) ProxyServer {
	log.Printf("Socket server started on port %v\n", appContext.port)
	access := configuration.NormalizeAccessControl(config.AccessControl, config.EnableRemoteConnection)
	if accessControl != nil {
		access = accessControl.Get().Proxy
	}
	addr := fmt.Sprintf("%s:%d", access.ListenHost(), appContext.port)
	if access.Mode == configuration.AccessControlLoopback {
		fmt.Printf("✅🔒 Proxy restricted to localhost on port %d\n", appContext.port)
	} else {
		fmt.Printf("❗🔓 Proxy listening on %s with %s access control\n", addr, access.Mode)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}

	return ProxyServer{
		listener:      listener,
		appContext:    appContext,
		EventLogger:   eventLogger,
		capture:       capture,
		store:         store,
		captureCtl:    captureCtl,
		accessControl: accessControl,
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
		if !s.allowsClient(browser.RemoteAddr()) {
			log.Printf("Rejected proxy connection from %s by access control\n", browser.RemoteAddr().String())
			_ = browser.Close()
			continue
		}
		fmt.Printf("New connection from %s\n", browser.RemoteAddr().String())
		requestId++
		go s.handleRequest(browser, requestId)(s.appContext.pipeline)
	}
}

func (s *ProxyServer) allowsClient(addr net.Addr) bool {
	if s.accessControl == nil {
		return true
	}
	return s.accessControl.AllowsProxy(addr)
}

func (s *ProxyServer) handleRequest(browser net.Conn, requestId int) func(pipeline middlewares.Middleware) {
	stream := http.NewNetworkStream(browser)
	request, err := http.ReadProxyRequest(stream)
	if err != nil {
		fmt.Printf("Error reading request from %s: %v\n", browser.RemoteAddr().String(), err)
		return func(pipeline middlewares.Middleware) {}
	}

	return func(pipeline middlewares.Middleware) {
		defer func() {
			_ = stream.Close()
		}()

		// One correlation id per request, shared between the real-time event
		// stream (SSE) and the persisted capture record, so the UI can link this
		// request to its response. A generation failure is non-fatal: the zero
		// UUID still yields a valid (all-zero) string.
		correlationID, err := storage.NewUUID()
		if err != nil {
			log.Printf("capture: failed to generate correlation id: %v\n", err)
		}

		pipelineToUse, decryptingHttps := snapshotPipeline(pipeline)

		// In decryption mode the CONNECT tunnel itself is not surfaced to the UI:
		// the HTTPS interceptor emits the decrypted requests/responses instead, so
		// showing the opaque CONNECT would only add a permanently-pending row.
		if s.isCapturing() && !(decryptingHttps && request.HttpRequestLine.IsConnect()) {
			s.EventLogger.LogRequest(requestId, correlationID.String(), request)
		}
		s.recordTopLevelRequest(correlationID, request, decryptingHttps)

		// Pass the buffered stream (not the raw connection) down the pipeline so
		// any request body bytes already pulled into the read buffer alongside
		// the headers are not lost when the pipeline forwards the connection.
		err = pipelineToUse.HandleProxyRequest(stream, request)
		if err != nil {
			fmt.Printf("Error handling request from %s: %v\n", browser.RemoteAddr().String(), err)
		}
	}
}

func snapshotPipeline(pipeline middlewares.Middleware) (middlewares.Middleware, bool) {
	if snapshotter, ok := pipeline.(pipelineSnapshotter); ok {
		return snapshotter.Snapshot()
	}
	return pipeline, false
}

// recordTopLevelRequest persists the proxied request line + headers to the
// capture file. In decryption mode the CONNECT tunnel is skipped here because
// the HTTPS interceptor records the decrypted requests/responses instead.
func (s *ProxyServer) recordTopLevelRequest(correlationID storage.UUID, request models.ProxyRequest, decryptingHttps bool) {
	if !s.isCapturing() {
		return
	}
	if s.capture == nil && s.store == nil {
		return
	}
	if decryptingHttps && request.HttpRequestLine.IsConnect() {
		return
	}
	rec := proxyRequestToRecord(correlationID, request)
	if s.capture != nil {
		if err := s.capture.WriteRequest(rec); err != nil {
			log.Printf("capture: failed to record request: %v\n", err)
		}
	}
	if s.store != nil {
		s.store.PutRequest(correlationID.String(), rec)
	}
}

func (s *ProxyServer) isCapturing() bool {
	return s.captureCtl == nil || s.captureCtl.IsCapturing()
}

// proxyRequestToRecord converts a parsed proxy request into a capture record,
// tagged with the shared correlation id so it lines up with the request event
// streamed to the UI. The body is not captured at this level (it is streamed by
// the pipeline); decrypted bodies are recorded by the HTTPS interceptor.
func proxyRequestToRecord(correlationID storage.UUID, request models.ProxyRequest) storage.RequestRecord {
	id := correlationID
	line := request.HttpRequestLine

	var url string
	if line.IsConnect() {
		url = fmt.Sprintf("%s:%d", line.Endpoint.Host, line.Endpoint.Port)
	} else {
		url = fmt.Sprintf("http://%s:%d%s", line.Endpoint.Host, line.Endpoint.Port, line.Endpoint.PathAndQuery)
	}

	headers := make([]storage.Header, 0, len(request.Headers))
	for _, h := range request.Headers {
		headers = append(headers, storage.Header{Name: h.Name, Value: h.Value})
	}

	return storage.RequestRecord{
		RequestID:   id,
		Method:      string(line.HttpMethod),
		URL:         url,
		HttpVersion: storage.NewHttpVersion(line.Version.Major, line.Version.Minor),
		Headers:     headers,
		Body:        nil,
	}
}
