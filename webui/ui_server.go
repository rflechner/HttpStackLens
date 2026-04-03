package webui

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"
)

type Hub struct {
	clients map[chan string]struct{}
	mu      sync.Mutex
}

func newHub() *Hub {
	return &Hub{clients: make(map[chan string]struct{})}
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

func (h *Hub) publish(eventType, data string) {
	msg := fmt.Sprintf("event: %s\ndata: %s", eventType, data)
	h.mu.Lock()
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
		// Headers obligatoires pour SSE
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

func ServeWebUi(port int) {
	rootFS := getFS()

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
		w.Write(data)
	})

	mux.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.FS(jsFS))))
	mux.Handle("/images/", http.StripPrefix("/images/", http.FileServer(http.FS(imagesFS))))
	mux.Handle("/wasm/", http.StripPrefix("/wasm/", http.FileServer(http.FS(wasmFS))))

	hub := newHub()
	mux.HandleFunc("/events", sseHandler(hub))

	ticker := time.NewTicker(1 * time.Second)

	go func() {
		for range ticker.C {
			hub.publish("request_occurred", "coucou")
		}
	}()

	addr := fmt.Sprintf("localhost:%d", port)
	fmt.Printf("Web UI start at http://%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
