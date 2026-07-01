package webui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"httpStackLens/certManager"
	configuration "httpStackLens/configuration"
	"httpStackLens/webui/wasm/shared"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"
)

type Hub struct {
	clients  map[chan string]struct{}
	mu       sync.Mutex
	shutdown chan bool
	closed   bool
}

func newHub() *Hub {
	h := &Hub{
		clients:  make(map[chan string]struct{}),
		shutdown: make(chan bool),
	}
	go h.run()
	return h
}

func (h *Hub) run() {
	for {
		select {
		case <-h.shutdown:
			h.mu.Lock()
			h.closed = true
			for client := range h.clients {
				fmt.Println("closing client")
				close(client)
				fmt.Println("closed client")
			}
			h.mu.Unlock()
			return
		}
	}
}

func (h *Hub) Close() {
	fmt.Println("closing hub")
	close(h.shutdown)
	fmt.Println("closed hub")
}

func (h *Hub) subscribe() chan string {
	ch := make(chan string, 8)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) unsubscribe(ch chan string) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

func (h *Hub) Publish(eventType, data string) {
	msg := fmt.Sprintf("event: %s\ndata: %s", eventType, data)
	h.mu.Lock()
	if h.closed {
		fmt.Println("hub is closed")
		return
	}
	for ch := range h.clients {
		select {
		case ch <- msg:
		default: // too slow clients are ignored
		}
	}
	h.mu.Unlock()
}

func sseHandler(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Mandatory headers for SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		ch := hub.subscribe()
		defer hub.unsubscribe(ch)
		log.Printf("[SSE] client connected — %s", r.RemoteAddr)

		for {
			select {
			case <-hub.shutdown:
				fmt.Println("[SSE] hub is closing => closing SSE")
				return
			case msg := <-ch:
				_, _ = fmt.Fprintf(w, "%s\n\n", msg)
				flusher.Flush()
			case <-r.Context().Done():
				log.Printf("[SSE] client disconnected — %s", r.RemoteAddr)
				return
			}
		}
	}
}

func ServeWebUi(port int, stop <-chan bool, config configuration.AppConfig) *Hub {
	rootFS := getFS()

	cssFS, err := fs.Sub(rootFS, "wwwroot/css")
	if err != nil {
		log.Fatal(err)
	}

	jsFS, err := fs.Sub(rootFS, "wwwroot/js")
	if err != nil {
		log.Fatal(err)
	}

	imagesFS, err := fs.Sub(rootFS, "wwwroot/images")
	if err != nil {
		log.Fatal(err)
	}

	wasmFS, err := fs.Sub(rootFS, "wwwroot/wasm")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(rootFS, "wwwroot/index.html")
		if err != nil {
			log.Printf("Request to %s failed: %v\n", r.URL, err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})

	mux.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.FS(cssFS))))
	mux.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.FS(jsFS))))
	mux.Handle("/images/", http.StripPrefix("/images/", http.FileServer(http.FS(imagesFS))))

	mux.HandleFunc("/wasm/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/wasm")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		http.StripPrefix("/wasm/", http.FileServer(http.FS(wasmFS))).ServeHTTP(w, r)
	})

	hub := newHub()
	mux.HandleFunc("/events", sseHandler(hub))

	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonData, err := json.Marshal(config.ToDto())
		if err != nil {
			log.Printf("Error marshaling request event: %v", err)
			return
		}
		_, _ = w.Write(jsonData)
	})

	mux.HandleFunc("/certificates-infos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		caCert, _, err := certManager.LoadCA(config.CertManager.CaCertFile, config.CertManager.CaKeyFile)
		if err != nil {
			log.Printf("Failed to load CA: %v\n", err)
			return
		}

		rs := shared.CertificatesInfosDto{
			CaCertSubject: caCert.Subject.CommonName,
		}
		jsonData, err := json.Marshal(rs)
		if err != nil {
			log.Printf("Error marshaling request event: %v", err)
			return
		}
		_, _ = w.Write(jsonData)
	})

	var addr string
	if config.WebUi.EnableRemoteConnection {
		addr = fmt.Sprintf("0.0.0.0:%d", port)
		fmt.Printf("❗️🔓 Web UI accepting remote connections on port %d\n", port)
	} else {
		addr = fmt.Sprintf("127.0.0.1:%d", port)
		fmt.Printf("✅🔒 Web UI restricted to localhost on port %d\n", port)
	}
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Start web server
	go func() {
		fmt.Printf("Web UI started at http://%s\n", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Web UI server error: %v", err)
		}
	}()

	go func() {
		<-stop // wait for stop signal

		log.Println("Shutting down Web UI server...")

		hub.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Web UI server shutdown error: %v", err)
		} else {
			log.Println("Web UI server stopped gracefully")
		}
	}()

	return hub
}
