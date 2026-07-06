package webui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"httpStackLens/certManager"
	configuration "httpStackLens/configuration"
	"httpStackLens/storage"
	"httpStackLens/webui/wasm/shared"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultCaptureRecordsLimit = 100
const maxCaptureRecordsLimit = 1000

type storageEnabledPersister func(bool) error
type bodyCaptureSettingsPersister func(configuration.DecryptHttpsConfig) error
type upstreamSettingsPersister func(configuration.UpstreamSettings) error

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

func ServeWebUi(port int, stop <-chan bool, config configuration.AppConfig, decryptHttpsSettings *configuration.DecryptHttpsConfigStore, upstreamSettings *configuration.UpstreamSettingsStore, requestStore *storage.RequestStore, captureCtl *storage.CaptureController, persistStorageEnabled storageEnabledPersister, persistBodyCaptureSettings bodyCaptureSettingsPersister, persistUpstreamSettings upstreamSettingsPersister) *Hub {
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
	mux.HandleFunc("/mockup", func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(rootFS, "wwwroot/mockup.html")
		if err != nil {
			log.Printf("Request to %s failed: %v\n", r.URL, err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/mockup.js", func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(rootFS, "wwwroot/mockup.js")
		if err != nil {
			log.Printf("Request to %s failed: %v\n", r.URL, err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(rootFS, "wwwroot/openapi.yaml")
		if err != nil {
			log.Printf("Request to %s failed: %v\n", r.URL, err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
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
	mux.HandleFunc("/api/requests/", requestsAPIHandler(requestStore))
	mux.HandleFunc("/api/capture/state", captureStateHandler(captureCtl, requestStore))
	mux.HandleFunc("/api/capture/pause", capturePauseHandler(hub, captureCtl, requestStore, persistStorageEnabled))
	mux.HandleFunc("/api/capture/resume", captureResumeHandler(hub, captureCtl, requestStore, persistStorageEnabled))
	mux.HandleFunc("/api/capture/clear", captureClearHandler(hub, captureCtl, requestStore))
	mux.HandleFunc("/api/captures", captureListHandler(config.Storage.Folder))
	mux.HandleFunc("/api/captures/", capturesAPIHandler(config.Storage.Folder))
	mux.HandleFunc("/api/settings/body-capture", bodyCaptureSettingsHandler(decryptHttpsSettings, persistBodyCaptureSettings))
	mux.HandleFunc("/api/settings/upstream", upstreamSettingsHandler(upstreamSettings, persistUpstreamSettings))

	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		dtoConfig := config
		if decryptHttpsSettings != nil {
			dtoConfig.DecryptHttps = decryptHttpsSettings.Get()
		}
		jsonData, err := json.Marshal(dtoConfig.ToDto())
		if err != nil {
			log.Printf("Error marshaling request event: %v", err)
			return
		}
		_, _ = w.Write(jsonData)
	})

	mux.HandleFunc("/certificates-infos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		certConfig := config.DecryptHttps.CertManager
		caCert, _, err := certManager.LoadCA(certConfig.CaCertFile, certConfig.CaKeyFile)
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

func captureStateHandler(captureCtl *storage.CaptureController, store *storage.RequestStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, captureStateDto(captureCtl, store))
	}
}

func capturePauseHandler(hub *Hub, captureCtl *storage.CaptureController, store *storage.RequestStore, persistStorageEnabled storageEnabledPersister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := persistCaptureSetting(persistStorageEnabled, false); err != nil {
			log.Printf("Error persisting storage.enable=false: %v", err)
			http.Error(w, "could not persist capture state", http.StatusInternalServerError)
			return
		}
		if captureCtl != nil {
			captureCtl.Pause()
		}
		state := captureStateDto(captureCtl, store)
		publishCaptureState(hub, state)
		writeJSON(w, state)
	}
}

func captureResumeHandler(hub *Hub, captureCtl *storage.CaptureController, store *storage.RequestStore, persistStorageEnabled storageEnabledPersister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := persistCaptureSetting(persistStorageEnabled, true); err != nil {
			log.Printf("Error persisting storage.enable=true: %v", err)
			http.Error(w, "could not persist capture state", http.StatusInternalServerError)
			return
		}
		if captureCtl != nil {
			captureCtl.Resume()
		}
		state := captureStateDto(captureCtl, store)
		publishCaptureState(hub, state)
		writeJSON(w, state)
	}
}

func persistCaptureSetting(persistStorageEnabled storageEnabledPersister, enabled bool) error {
	if persistStorageEnabled == nil {
		return nil
	}
	return persistStorageEnabled(enabled)
}

func captureClearHandler(hub *Hub, captureCtl *storage.CaptureController, store *storage.RequestStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if store != nil {
			store.Clear()
		}
		state := captureStateDto(captureCtl, store)
		publishCaptureState(hub, state)
		writeJSON(w, state)
	}
}

func captureStateDto(captureCtl *storage.CaptureController, store *storage.RequestStore) shared.CaptureStateDto {
	size := 0
	if store != nil {
		size = store.Len()
	}
	capturing := true
	if captureCtl != nil {
		capturing = captureCtl.IsCapturing()
	}
	return shared.CaptureStateDto{Capturing: capturing, BufferSize: size}
}

func publishCaptureState(hub *Hub, state shared.CaptureStateDto) {
	if hub == nil {
		return
	}
	jsonData, err := json.Marshal(state)
	if err != nil {
		log.Printf("Error marshaling capture state: %v", err)
		return
	}
	hub.Publish("capture_state", string(jsonData))
}

func bodyCaptureSettingsHandler(settings *configuration.DecryptHttpsConfigStore, persist bodyCaptureSettingsPersister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if settings == nil {
			http.Error(w, "body capture settings are unavailable", http.StatusServiceUnavailable)
			return
		}

		switch r.Method {
		case http.MethodGet:
			writeJSON(w, bodyCaptureSettingsDto(settings.Get()))
		case http.MethodPut:
			var dto shared.BodyCaptureSettingsDto
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&dto); err != nil {
				http.Error(w, "invalid body capture settings", http.StatusBadRequest)
				return
			}
			rules, err := mimeTypeRulesFromDto(dto.MimeTypes)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if dto.DefaultMaxBytes != nil && *dto.DefaultMaxBytes < 0 {
				http.Error(w, "default_max_bytes must be greater than or equal to 0", http.StatusBadRequest)
				return
			}

			next := settings.Get()
			next.DefaultMaxBytes = dto.DefaultMaxBytes
			next.MimeTypes = rules
			if persist != nil {
				if err := persist(next); err != nil {
					log.Printf("Error persisting decrypt_https body capture rules: %v", err)
					http.Error(w, "could not persist body capture settings", http.StatusInternalServerError)
					return
				}
			}
			updated := settings.UpdateCaptureRules(dto.DefaultMaxBytes, rules)
			writeJSON(w, bodyCaptureSettingsDto(updated))
		default:
			w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodPut}, ", "))
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func upstreamSettingsHandler(settings *configuration.UpstreamSettingsStore, persist upstreamSettingsPersister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if settings == nil {
			http.Error(w, "upstream settings are unavailable", http.StatusServiceUnavailable)
			return
		}

		switch r.Method {
		case http.MethodGet:
			writeJSON(w, upstreamSettingsDto(settings.Get()))
		case http.MethodPut:
			var dto shared.UpstreamSettingsDto
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&dto); err != nil {
				http.Error(w, "invalid upstream settings", http.StatusBadRequest)
				return
			}
			next, err := upstreamSettingsFromDto(dto)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if persist != nil {
				if err := persist(next); err != nil {
					log.Printf("Error persisting upstream proxy settings: %v", err)
					http.Error(w, "could not persist upstream settings", http.StatusInternalServerError)
					return
				}
			}
			updated := settings.Update(next)
			writeJSON(w, upstreamSettingsDto(updated))
		default:
			w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodPut}, ", "))
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func upstreamSettingsDto(settings configuration.UpstreamSettings) shared.UpstreamSettingsDto {
	return shared.UpstreamSettingsDto{
		OutputProxyUri:           settings.OutputProxyUri,
		NoProxy:                  settings.NoProxy,
		AddWindowsAuthentication: settings.AddWindowsAuthentication,
	}
}

func upstreamSettingsFromDto(dto shared.UpstreamSettingsDto) (configuration.UpstreamSettings, error) {
	uri := strings.TrimSpace(dto.OutputProxyUri)
	if uri != "" {
		parsed, err := url.Parse(uri)
		if err != nil {
			return configuration.UpstreamSettings{}, fmt.Errorf("output_proxy_uri is not a valid URL: %v", err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return configuration.UpstreamSettings{}, fmt.Errorf("output_proxy_uri must include a scheme and host")
		}
	}

	var noProxy []string
	for _, host := range dto.NoProxy {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		noProxy = append(noProxy, host)
	}

	return configuration.UpstreamSettings{
		OutputProxyUri:           uri,
		NoProxy:                  noProxy,
		AddWindowsAuthentication: dto.AddWindowsAuthentication,
	}, nil
}

func bodyCaptureSettingsDto(config configuration.DecryptHttpsConfig) shared.BodyCaptureSettingsDto {
	return shared.BodyCaptureSettingsDto{
		DefaultMaxBytes: config.DefaultMaxBytes,
		MimeTypes:       mimeTypeRulesToDto(config.MimeTypes),
	}
}

func mimeTypeRulesToDto(rules []configuration.MimeTypeRule) []shared.MimeTypeRuleDto {
	if rules == nil {
		return nil
	}
	out := make([]shared.MimeTypeRuleDto, len(rules))
	for i, rule := range rules {
		out[i] = shared.MimeTypeRuleDto{
			Name:         rule.Name,
			MaxSizeBytes: rule.MaxSizeBytes,
			MaxSizeKb:    rule.MaxSizeKb,
			MaxSizeMb:    rule.MaxSizeMb,
		}
	}
	return out
}

func mimeTypeRulesFromDto(rules []shared.MimeTypeRuleDto) ([]configuration.MimeTypeRule, error) {
	if rules == nil {
		return nil, nil
	}
	out := make([]configuration.MimeTypeRule, len(rules))
	for i, rule := range rules {
		name := strings.TrimSpace(rule.Name)
		if name == "" {
			return nil, fmt.Errorf("mime_types[%d].name is required", i)
		}
		if !strings.Contains(name, "/") {
			return nil, fmt.Errorf("mime_types[%d].name must be a MIME type or wildcard", i)
		}
		sizeFields := 0
		if rule.MaxSizeBytes != nil {
			sizeFields++
			if *rule.MaxSizeBytes < 0 {
				return nil, fmt.Errorf("mime_types[%d].max_size_bytes must be greater than or equal to 0", i)
			}
		}
		if rule.MaxSizeKb != nil {
			sizeFields++
			if *rule.MaxSizeKb < 0 {
				return nil, fmt.Errorf("mime_types[%d].max_size_kb must be greater than or equal to 0", i)
			}
		}
		if rule.MaxSizeMb != nil {
			sizeFields++
			if *rule.MaxSizeMb < 0 {
				return nil, fmt.Errorf("mime_types[%d].max_size_mb must be greater than or equal to 0", i)
			}
		}
		if sizeFields > 1 {
			return nil, fmt.Errorf("mime_types[%d] must specify at most one size field", i)
		}
		out[i] = configuration.MimeTypeRule{
			Name:         name,
			MaxSizeBytes: rule.MaxSizeBytes,
			MaxSizeKb:    rule.MaxSizeKb,
			MaxSizeMb:    rule.MaxSizeMb,
		}
	}
	return out, nil
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("Error marshaling JSON response: %v", err)
	}
}

func captureListHandler(folder string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/captures" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		files, err := listCaptureFiles(captureFolder(folder))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				files = nil
			} else {
				log.Printf("Error listing capture files: %v", err)
				http.Error(w, "could not list captures", http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(files); err != nil {
			log.Printf("Error marshaling capture list: %v", err)
		}
	}
}

func capturesAPIHandler(folder string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		name, action, ok := parseCaptureAPIPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}

		path, err := captureFilePath(captureFolder(folder), name)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		switch action {
		case "metadata":
			captureMetadataHandler(path, name).ServeHTTP(w, r)
		case "records":
			captureRecordsHandler(path, name).ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	}
}

func captureMetadataHandler(path, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info, header, err := readCaptureMetadata(path)
		if err != nil {
			writeCaptureReadError(w, err)
			return
		}

		dto := shared.CaptureMetadataDto{
			Name:           name,
			Size:           info.Size(),
			ModifiedAt:     info.ModTime().UTC().Format(time.RFC3339Nano),
			Version:        header.Version,
			HttpsDecrypted: header.HttpsDecrypted,
			RecordsCount:   header.RecordsCount,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(dto); err != nil {
			log.Printf("Error marshaling capture metadata: %v", err)
		}
	}
}

func captureRecordsHandler(path, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		offset, limit, err := parseCaptureRecordPage(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		records, nextOffset, hasMore, err := readCaptureRecordsPage(path, offset, limit)
		if err != nil {
			writeCaptureReadError(w, err)
			return
		}

		dto := shared.CaptureRecordsDto{
			Name:       name,
			Offset:     offset,
			Limit:      limit,
			Records:    records,
			NextOffset: nextOffset,
			HasMore:    hasMore,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(dto); err != nil {
			log.Printf("Error marshaling capture records: %v", err)
		}
	}
}

func requestsAPIHandler(store *storage.RequestStore) http.HandlerFunc {
	detail := requestDetailHandler(store)
	body := requestBodyHandler(store)
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/body") {
			body(w, r)
			return
		}
		detail(w, r)
	}
}

func captureFolder(folder string) string {
	if folder == "" {
		return "captures"
	}
	return folder
}

func listCaptureFiles(folder string) ([]shared.CaptureFileDto, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}

	files := make([]shared.CaptureFileDto, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".capture" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		files = append(files, shared.CaptureFileDto{
			Name:       entry.Name(),
			Size:       info.Size(),
			ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].ModifiedAt > files[j].ModifiedAt
	})
	return files, nil
}

func parseCaptureAPIPath(path string) (name, action string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/captures/")
	if rest == path || rest == "" {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	if parts[1] != "metadata" && parts[1] != "records" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func captureFilePath(folder, name string) (string, error) {
	if name == "" || name != filepath.Base(name) || filepath.Ext(name) != ".capture" {
		return "", fmt.Errorf("invalid capture name")
	}
	return filepath.Join(folder, name), nil
}

func readCaptureMetadata(path string) (os.FileInfo, storage.FileHeader, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, storage.FileHeader{}, err
	}
	if info.IsDir() {
		return nil, storage.FileHeader{}, os.ErrNotExist
	}

	reader, err := storage.NewFileCaptureSessionReader(path)
	if err != nil {
		return nil, storage.FileHeader{}, err
	}
	defer func() { _ = reader.Close() }()

	header := reader.Header()
	if header.RecordsCount < 0 {
		count, err := countCaptureRecords(path)
		if err != nil {
			return nil, storage.FileHeader{}, err
		}
		header.RecordsCount = count
	}
	return info, header, nil
}

func countCaptureRecords(path string) (int32, error) {
	reader, err := storage.NewFileCaptureSessionReader(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = reader.Close() }()

	var count int32
	for {
		_, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return count, nil
			}
			return 0, err
		}
		count++
	}
}

func parseCaptureRecordPage(r *http.Request) (offset, limit int, err error) {
	offset, err = parseNonNegativeIntQuery(r, "offset", 0)
	if err != nil {
		return 0, 0, err
	}
	limit, err = parseNonNegativeIntQuery(r, "limit", defaultCaptureRecordsLimit)
	if err != nil {
		return 0, 0, err
	}
	if limit < 1 {
		return 0, 0, fmt.Errorf("limit must be greater than zero")
	}
	if limit > maxCaptureRecordsLimit {
		limit = maxCaptureRecordsLimit
	}
	return offset, limit, nil
}

func parseNonNegativeIntQuery(r *http.Request, name string, defaultValue int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return value, nil
}

func readCaptureRecordsPage(path string, offset, limit int) ([]shared.CaptureRecordDto, int, bool, error) {
	reader, err := storage.NewFileCaptureSessionReader(path)
	if err != nil {
		return nil, 0, false, err
	}
	defer func() { _ = reader.Close() }()

	records := make([]shared.CaptureRecordDto, 0, limit)
	index := 0
	for {
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return records, index, false, nil
			}
			return nil, 0, false, err
		}

		if index >= offset {
			if len(records) >= limit {
				return records, index, true, nil
			}
			records = append(records, captureRecordDto(index, record))
		}
		index++
	}
}

func captureRecordDto(index int, record storage.CaptureRecord) shared.CaptureRecordDto {
	switch r := record.(type) {
	case storage.RequestRecord:
		return shared.CaptureRecordDto{
			Index: index,
			Type:  "request",
			Request: &shared.CaptureRequestRecordDto{
				RequestID:     r.RequestID.String(),
				Method:        r.Method,
				URL:           r.URL,
				HttpVersion:   httpVersionString(r.HttpVersion),
				Headers:       headerDtos(r.Headers),
				BodySkipped:   r.BodySkipped,
				BodyAvailable: r.Body != nil && !r.BodySkipped,
				BodySize:      len(r.Body),
				BodyBase64:    bodyBase64(r.Body, r.BodySkipped),
			},
		}
	case storage.ResponseRecord:
		return shared.CaptureRecordDto{
			Index: index,
			Type:  "response",
			Response: &shared.CaptureResponseRecordDto{
				RequestID:     r.RequestID.String(),
				Status:        int(r.StatusCode),
				StatusText:    r.StatusMessage,
				HttpVersion:   httpVersionString(r.HttpVersion),
				Headers:       headerDtos(r.Headers),
				BodySkipped:   r.BodySkipped,
				BodyAvailable: r.Body != nil && !r.BodySkipped,
				BodySize:      len(r.Body),
				BodyBase64:    bodyBase64(r.Body, r.BodySkipped),
			},
		}
	default:
		return shared.CaptureRecordDto{Index: index, Type: "unknown"}
	}
}

func bodyBase64(body []byte, skipped bool) string {
	if body == nil || skipped {
		return ""
	}
	return base64.StdEncoding.EncodeToString(body)
}

func writeCaptureReadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, os.ErrNotExist):
		http.Error(w, "capture not found", http.StatusNotFound)
	case errors.Is(err, storage.ErrBadMagic), errors.Is(err, storage.ErrUnsupportedVersion):
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	default:
		log.Printf("Error reading capture file: %v", err)
		http.Error(w, "could not read capture", http.StatusInternalServerError)
	}
}

func requestDetailHandler(store *storage.RequestStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if store == nil {
			http.Error(w, "request store unavailable", http.StatusServiceUnavailable)
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/requests/")
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}

		exchange, ok := store.Get(id)
		if !ok {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(requestDetailDto(exchange)); err != nil {
			log.Printf("Error marshaling request detail: %v", err)
		}
	}
}

func requestBodyHandler(store *storage.RequestStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if store == nil {
			http.Error(w, "request store unavailable", http.StatusServiceUnavailable)
			return
		}

		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/requests/"), "/body")
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}

		side := r.URL.Query().Get("side")
		if side != "request" && side != "response" {
			http.Error(w, "side must be request or response", http.StatusBadRequest)
			return
		}

		exchange, ok := store.Get(id)
		if !ok {
			http.NotFound(w, r)
			return
		}

		body, contentType, skipped, ok := bodyForSide(exchange, side)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if skipped {
			w.Header().Set("X-Body-Skipped", "true")
			http.Error(w, "body was not captured", http.StatusRequestEntityTooLarge)
			return
		}

		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(body)
	}
}

func requestDetailDto(exchange storage.CapturedExchange) shared.RequestDetailDto {
	dto := shared.RequestDetailDto{
		CorrelationID: exchange.CorrelationID,
		CreatedAt:     exchange.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if exchange.Request != nil {
		dto.Request = &shared.RequestDetailRequestDto{
			Method:        exchange.Request.Method,
			URL:           exchange.Request.URL,
			HttpVersion:   httpVersionString(exchange.Request.HttpVersion),
			Headers:       headerDtos(exchange.Request.Headers),
			BodyAvailable: exchange.Request.Body != nil && !exchange.Request.BodySkipped,
			BodySkipped:   exchange.Request.BodySkipped,
			BodySize:      len(exchange.Request.Body),
		}
	}
	if exchange.Response != nil {
		dto.Response = &shared.RequestDetailResponseDto{
			Status:        int(exchange.Response.StatusCode),
			StatusText:    exchange.Response.StatusMessage,
			HttpVersion:   httpVersionString(exchange.Response.HttpVersion),
			Headers:       headerDtos(exchange.Response.Headers),
			BodyAvailable: exchange.Response.Body != nil && !exchange.Response.BodySkipped,
			BodySkipped:   exchange.Response.BodySkipped,
			BodySize:      len(exchange.Response.Body),
		}
	}
	if exchange.Timing != nil {
		dto.Timing = timingDto(*exchange.Timing)
	}
	return dto
}

// timingDto converts the stored per-phase durations into the millisecond DTO
// consumed by the UI's Timing tab / waterfall.
func timingDto(t storage.Timing) *shared.TimingDto {
	return &shared.TimingDto{
		DnsMs:      t.Dns.Milliseconds(),
		ConnectMs:  t.Connect.Milliseconds(),
		TlsMs:      t.Tls.Milliseconds(),
		TtfbMs:     t.Ttfb.Milliseconds(),
		DownloadMs: t.Download.Milliseconds(),
		TotalMs:    t.Total.Milliseconds(),
	}
}

func bodyForSide(exchange storage.CapturedExchange, side string) (body []byte, contentType string, skipped bool, ok bool) {
	switch side {
	case "request":
		if exchange.Request == nil {
			return nil, "", false, false
		}
		if exchange.Request.BodySkipped {
			return nil, "", true, true
		}
		if exchange.Request.Body == nil {
			return nil, "", false, false
		}
		return exchange.Request.Body, contentTypeFromHeaders(exchange.Request.Headers), false, true
	case "response":
		if exchange.Response == nil {
			return nil, "", false, false
		}
		if exchange.Response.BodySkipped {
			return nil, "", true, true
		}
		if exchange.Response.Body == nil {
			return nil, "", false, false
		}
		return exchange.Response.Body, contentTypeFromHeaders(exchange.Response.Headers), false, true
	default:
		return nil, "", false, false
	}
}

func contentTypeFromHeaders(headers []storage.Header) string {
	for _, h := range headers {
		if strings.EqualFold(h.Name, "Content-Type") && h.Value != "" {
			return h.Value
		}
	}
	return "application/octet-stream"
}

func headerDtos(headers []storage.Header) []shared.HeaderDto {
	if len(headers) == 0 {
		return nil
	}
	out := make([]shared.HeaderDto, 0, len(headers))
	for _, h := range headers {
		out = append(out, shared.HeaderDto{Name: h.Name, Value: h.Value})
	}
	return out
}

func httpVersionString(v storage.HttpVersion) string {
	if v == storage.HttpVersionUnknown {
		return ""
	}
	return fmt.Sprintf("HTTP/%d.%d", v.Major(), v.Minor())
}
