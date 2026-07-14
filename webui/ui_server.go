package webui

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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
type accessControlSettingsPersister func(configuration.AccessControlSettings) error
type decryptHttpsToggleUpdater func(bool) (configuration.DecryptHttpsConfig, error)
type proxyRuntimeUpdater func(bool) (bool, error)

type RuntimeCommandKind int

const (
	SetStorageEnabled RuntimeCommandKind = iota
	SetBodyCapture
	SetDecryptHTTPS
	SetUpstream
	SetAccessControl
	StartProxy
	StopProxy
)

// RuntimeCommand is emitted by the HTTP adapter and handled by the application
// supervisor. Reply is buffered so the supervisor never remains blocked if the
// originating HTTP request is cancelled while a command is being applied.
type RuntimeCommand struct {
	Kind          RuntimeCommandKind
	Enabled       bool
	DecryptHTTPS  configuration.DecryptHttpsConfig
	Upstream      configuration.UpstreamSettings
	AccessControl configuration.AccessControlSettings
	Reply         chan RuntimeCommandResult
}

type RuntimeCommandResult struct {
	DecryptHTTPS configuration.DecryptHttpsConfig
	ProxyRunning bool
	Err          error
}

type Dependencies struct {
	InitialConfig         configuration.AppConfig
	CurrentConfig         func() configuration.AppConfig
	DecryptHTTPSSettings  *configuration.DecryptHttpsConfigStore
	UpstreamSettings      *configuration.UpstreamSettingsStore
	AccessControlSettings *configuration.AccessControlSettingsStore
	Requests              *storage.RequestStore
	Capture               *storage.CaptureController
	Proxy                 *storage.ProxyController
	Commands              chan<- RuntimeCommand
}

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

func ServeWebUi(port int, stop <-chan bool, deps Dependencies) *Hub {
	config := deps.InitialConfig
	decryptHttpsSettings := deps.DecryptHTTPSSettings
	upstreamSettings := deps.UpstreamSettings
	accessControlSettings := deps.AccessControlSettings
	requestStore := deps.Requests
	captureCtl := deps.Capture
	proxyCtl := deps.Proxy

	send := func(command RuntimeCommand) RuntimeCommandResult {
		if deps.Commands == nil {
			return RuntimeCommandResult{Err: errors.New("runtime configuration is unavailable")}
		}
		command.Reply = make(chan RuntimeCommandResult, 1)
		deps.Commands <- command
		return <-command.Reply
	}
	persistBodyCaptureSettings := func(settings configuration.DecryptHttpsConfig) error {
		return send(RuntimeCommand{Kind: SetBodyCapture, DecryptHTTPS: settings}).Err
	}
	persistUpstreamSettings := func(settings configuration.UpstreamSettings) error {
		return send(RuntimeCommand{Kind: SetUpstream, Upstream: settings}).Err
	}
	persistAccessControlSettings := func(settings configuration.AccessControlSettings) error {
		return send(RuntimeCommand{Kind: SetAccessControl, AccessControl: settings}).Err
	}
	updateDecryptHttps := func(enabled bool) (configuration.DecryptHttpsConfig, error) {
		result := send(RuntimeCommand{Kind: SetDecryptHTTPS, Enabled: enabled})
		return result.DecryptHTTPS, result.Err
	}
	updateProxy := func(running bool) (bool, error) {
		kind := StopProxy
		if running {
			kind = StartProxy
		}
		result := send(RuntimeCommand{Kind: kind})
		return result.ProxyRunning, result.Err
	}
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
	mux.HandleFunc("/ui.js", func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(rootFS, "wwwroot/ui.js")
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
	// captureState is the single source of truth for the capture_state SSE event
	// and the /api/capture/state endpoint. It bundles the capture flag with the
	// live decrypt/upstream/access states so the status bar (F3.2) stays in sync.
	captureState := func() shared.CaptureStateDto {
		return captureStateDto(captureCtl, requestStore, decryptHttpsSettings, upstreamSettings, accessControlSettings, proxyCtl)
	}
	broadcastCaptureState := func() { publishCaptureState(hub, captureState()) }
	mux.HandleFunc("/events", sseHandler(hub))
	mux.HandleFunc("/api/requests/", requestsAPIHandler(requestStore))
	mux.HandleFunc("/api/capture/state", captureStateHandler(captureState))
	mux.HandleFunc("/api/capture/pause", capturePauseHandler(hub, captureCtl, nil, captureState))
	mux.HandleFunc("/api/capture/resume", captureResumeHandler(hub, captureCtl, nil, captureState))
	mux.HandleFunc("/api/capture/clear", captureClearHandler(hub, requestStore, captureState))
	// Preferred recording terminology. The capture routes remain as compatible
	// aliases for existing clients.
	mux.HandleFunc("/api/recording/state", captureStateHandler(captureState))
	mux.HandleFunc("/api/recording/start", captureResumeHandler(hub, captureCtl, nil, captureState))
	mux.HandleFunc("/api/recording/stop", capturePauseHandler(hub, captureCtl, nil, captureState))
	mux.HandleFunc("/api/recording/clear", captureClearHandler(hub, requestStore, captureState))
	mux.HandleFunc("/api/proxy/start", proxyRuntimeHandler(true, hub, updateProxy, captureState))
	mux.HandleFunc("/api/proxy/stop", proxyRuntimeHandler(false, hub, updateProxy, captureState))
	mux.HandleFunc("/api/proxy/state", captureStateHandler(captureState))
	mux.HandleFunc("/api/runtime/stats", runtimeStatsHandler)
	mux.HandleFunc("/api/captures", captureListHandler(config.Storage.Folder))
	mux.HandleFunc("/api/captures/", capturesAPIHandler(config.Storage.Folder))
	mux.HandleFunc("/api/settings/body-capture", bodyCaptureSettingsHandler(decryptHttpsSettings, persistBodyCaptureSettings))
	mux.HandleFunc("/api/settings/decrypt-https", decryptHttpsToggleHandler(decryptHttpsSettings, updateDecryptHttps, broadcastCaptureState))
	mux.HandleFunc("/api/settings/upstream", upstreamSettingsHandler(upstreamSettings, persistUpstreamSettings, broadcastCaptureState))
	mux.HandleFunc("/api/settings/access-control", accessControlSettingsHandler(accessControlSettings, persistAccessControlSettings, broadcastCaptureState))

	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		dtoConfig := config
		if deps.CurrentConfig != nil {
			dtoConfig = deps.CurrentConfig()
		}
		if decryptHttpsSettings != nil {
			dtoConfig.DecryptHttps = decryptHttpsSettings.Get()
		}
		if accessControlSettings != nil {
			access := accessControlSettings.Get()
			dtoConfig.Proxy.AccessControl = access.Proxy
			dtoConfig.WebUi.AccessControl = access.WebUi
		}
		jsonData, err := json.Marshal(dtoConfig.ToDto())
		if err != nil {
			log.Printf("Error marshaling request event: %v", err)
			return
		}
		_, _ = w.Write(jsonData)
	})

	certInstaller := certManager.NewCertInstaller()
	mux.HandleFunc("/certificates-infos", certificatesInfosHandler(config.DecryptHttps.CertManager, certInstaller))
	mux.HandleFunc("/api/certificates/ca/generate", certificateGenerateHandler(config.DecryptHttps.CertManager, certInstaller))
	mux.HandleFunc("/api/certificates/ca/install", certificateInstallHandler(config.DecryptHttps.CertManager, certInstaller))
	mux.HandleFunc("/api/certificates/ca/export", certificateExportHandler(config.DecryptHttps.CertManager))

	access := configuration.NormalizeAccessControl(config.WebUi.AccessControl, config.WebUi.EnableRemoteConnection)
	if accessControlSettings != nil {
		access = accessControlSettings.Get().WebUi
	}
	addr := fmt.Sprintf("%s:%d", access.ListenHost(), port)
	if access.Mode == configuration.AccessControlLoopback {
		fmt.Printf("✅🔒 Web UI restricted to localhost on port %d\n", port)
	} else {
		fmt.Printf("❗️🔓 Web UI listening on %s with %s access control\n", addr, access.Mode)
	}
	server := &http.Server{
		Addr:    addr,
		Handler: webUiAccessControlMiddleware(accessControlSettings, mux),
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

func captureStateHandler(stateFn func() shared.CaptureStateDto) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, stateFn())
	}
}

func capturePauseHandler(hub *Hub, captureCtl *storage.CaptureController, persistStorageEnabled storageEnabledPersister, stateFn func() shared.CaptureStateDto) http.HandlerFunc {
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
		state := stateFn()
		publishCaptureState(hub, state)
		writeJSON(w, state)
	}
}

func captureResumeHandler(hub *Hub, captureCtl *storage.CaptureController, persistStorageEnabled storageEnabledPersister, stateFn func() shared.CaptureStateDto) http.HandlerFunc {
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
		state := stateFn()
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

func captureClearHandler(hub *Hub, store *storage.RequestStore, stateFn func() shared.CaptureStateDto) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if store != nil {
			store.Clear()
		}
		state := stateFn()
		publishCaptureState(hub, state)
		writeJSON(w, state)
	}
}

func captureStateDto(captureCtl *storage.CaptureController, store *storage.RequestStore, decryptSettings *configuration.DecryptHttpsConfigStore, upstreamSettings *configuration.UpstreamSettingsStore, accessSettings *configuration.AccessControlSettingsStore, proxyControllers ...*storage.ProxyController) shared.CaptureStateDto {
	size := 0
	if store != nil {
		size = store.Len()
	}
	capturing := true
	if captureCtl != nil {
		capturing = captureCtl.IsCapturing()
	}
	dto := shared.CaptureStateDto{Capturing: capturing, Recording: capturing, BufferSize: size}
	if len(proxyControllers) > 0 && proxyControllers[0] != nil {
		dto.Proxy.Running = proxyControllers[0].IsRunning()
		dto.Proxy.Address = proxyControllers[0].Address()
	}
	if decryptSettings != nil {
		dto.Decrypt.Enabled = decryptSettings.Get().Enabled
	}
	if upstreamSettings != nil {
		up := upstreamSettings.Get()
		dto.Upstream.Enabled = strings.TrimSpace(up.OutputProxyUri) != ""
		dto.Upstream.Ntlm = up.AddWindowsAuthentication
	}
	if accessSettings != nil {
		dto.Access.Mode = string(accessSettings.Get().Proxy.Mode)
	}
	return dto
}

func proxyRuntimeHandler(start bool, hub *Hub, update proxyRuntimeUpdater, stateFn func() shared.CaptureStateDto) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if update == nil {
			http.Error(w, "proxy runtime control is unavailable", http.StatusServiceUnavailable)
			return
		}
		if _, err := update(start); err != nil {
			log.Printf("Could not change proxy runtime state: %v", err)
			http.Error(w, "could not change proxy runtime state", http.StatusInternalServerError)
			return
		}
		state := shared.CaptureStateDto{}
		if stateFn != nil {
			state = stateFn()
		}
		publishCaptureState(hub, state)
		writeJSON(w, state)
	}
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

func webUiAccessControlMiddleware(settings *configuration.AccessControlSettingsStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if settings != nil {
			addr, err := net.ResolveTCPAddr("tcp", r.RemoteAddr)
			if err != nil || !settings.AllowsWebUi(addr) {
				log.Printf("Rejected Web UI request from %s by access control\n", r.RemoteAddr)
				http.Error(w, "forbidden by access control", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func certificatesInfosHandler(certConfig configuration.CertManagerConfig, installer certManager.CertInstaller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		dto := certificatesInfosDto(certConfig, installer)
		writeJSON(w, dto)
	}
}

func certificateGenerateHandler(certConfig configuration.CertManagerConfig, installer certManager.CertInstaller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		replace, err := readCertificateGenerateRequest(r)
		if err != nil {
			http.Error(w, "invalid certificate generation request", http.StatusBadRequest)
			return
		}
		if err := validateCAConfig(certConfig); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !replace && localFileExists(certConfig.CaCertFile) && localFileExists(certConfig.CaKeyFile) {
			http.Error(w, "CA already exists; set replace=true to regenerate it", http.StatusConflict)
			return
		}
		if err := certManager.GenerateCA(certConfig.CaCertFile, certConfig.CaKeyFile); err != nil {
			log.Printf("Error generating local CA: %v", err)
			http.Error(w, "could not generate CA", http.StatusInternalServerError)
			return
		}
		writeJSON(w, certificatesInfosDto(certConfig, installer))
	}
}

func certificateInstallHandler(certConfig configuration.CertManagerConfig, installer certManager.CertInstaller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := validateCAConfig(certConfig); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if installer == nil || !installer.IsSupported() {
			http.Error(w, "CA installation is not supported on this operating system", http.StatusServiceUnavailable)
			return
		}
		if _, _, err := certManager.LoadCA(certConfig.CaCertFile, certConfig.CaKeyFile); err != nil {
			http.Error(w, "CA certificate/key could not be loaded", http.StatusBadRequest)
			return
		}
		if err := installer.InstallCACert(certConfig.CaCertFile); err != nil {
			log.Printf("Error installing local CA: %v", err)
			http.Error(w, "could not install CA", http.StatusInternalServerError)
			return
		}
		writeJSON(w, certificatesInfosDto(certConfig, installer))
	}
}

func certificateExportHandler(certConfig configuration.CertManagerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := validateCAConfig(certConfig); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if _, _, err := certManager.LoadCA(certConfig.CaCertFile, certConfig.CaKeyFile); err != nil {
			http.Error(w, "CA certificate/key could not be loaded", http.StatusNotFound)
			return
		}
		data, err := os.ReadFile(certConfig.CaCertFile)
		if err != nil {
			log.Printf("Error exporting local CA: %v", err)
			http.Error(w, "could not export CA", http.StatusInternalServerError)
			return
		}
		name := filepath.Base(certConfig.CaCertFile)
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, strings.ReplaceAll(name, `"`, "")))
		_, _ = w.Write(data)
	}
}

func readCertificateGenerateRequest(r *http.Request) (bool, error) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return false, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return false, nil
	}
	var dto shared.CertificateGenerateRequestDto
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&dto); err != nil {
		return false, err
	}
	return dto.Replace, nil
}

func validateCAConfig(certConfig configuration.CertManagerConfig) error {
	if certConfig.CaCertFile == "" || certConfig.CaKeyFile == "" {
		return errors.New("CA certificate and key files must be configured")
	}
	return nil
}

func localFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func certificatesInfosDto(certConfig configuration.CertManagerConfig, installer certManager.CertInstaller) shared.CertificatesInfosDto {
	dto := shared.CertificatesInfosDto{}
	if installer != nil {
		dto.InstallSupported = installer.IsSupported()
	}

	caCert, _, err := certManager.LoadCA(certConfig.CaCertFile, certConfig.CaKeyFile)
	if err != nil {
		dto.Error = err.Error()
		return dto
	}

	now := time.Now()
	dto.Available = true
	dto.CaCertSubject = caCert.Subject.String()
	dto.CaCertIssuer = caCert.Issuer.String()
	dto.CaCertSerialNumber = caCert.SerialNumber.String()
	dto.FingerprintSha256 = colonHex(sha256.Sum256(caCert.Raw))
	dto.NotBefore = caCert.NotBefore.UTC().Format(time.RFC3339Nano)
	dto.NotAfter = caCert.NotAfter.UTC().Format(time.RFC3339Nano)
	dto.Expired = now.Before(caCert.NotBefore) || now.After(caCert.NotAfter)

	if installer != nil && installer.IsSupported() {
		installed, err := installer.IsCACertInstalled(certConfig.CaCertFile)
		dto.Installed = installed
		if err != nil {
			dto.InstallCheckError = err.Error()
		}
	}
	return dto
}

func colonHex(sum [32]byte) string {
	encoded := strings.ToUpper(hex.EncodeToString(sum[:]))
	parts := make([]string, 0, len(encoded)/2)
	for i := 0; i < len(encoded); i += 2 {
		parts = append(parts, encoded[i:i+2])
	}
	return strings.Join(parts, ":")
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

func decryptHttpsToggleHandler(settings *configuration.DecryptHttpsConfigStore, update decryptHttpsToggleUpdater, broadcast func()) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if settings == nil {
			http.Error(w, "HTTPS decryption settings are unavailable", http.StatusServiceUnavailable)
			return
		}

		switch r.Method {
		case http.MethodGet:
			writeJSON(w, decryptHttpsToggleSettingsDto(settings.Get()))
		case http.MethodPut:
			if update == nil {
				http.Error(w, "HTTPS decryption toggle is unavailable", http.StatusServiceUnavailable)
				return
			}
			var dto shared.DecryptHttpsToggleSettingsDto
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&dto); err != nil {
				http.Error(w, "invalid HTTPS decryption settings", http.StatusBadRequest)
				return
			}
			updated, err := update(dto.Enabled)
			if err != nil {
				log.Printf("Error updating HTTPS decryption setting: %v", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if broadcast != nil {
				broadcast()
			}
			writeJSON(w, decryptHttpsToggleSettingsDto(updated))
		default:
			w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodPut}, ", "))
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func accessControlSettingsHandler(settings *configuration.AccessControlSettingsStore, persist accessControlSettingsPersister, broadcast func()) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if settings == nil {
			http.Error(w, "access control settings are unavailable", http.StatusServiceUnavailable)
			return
		}

		switch r.Method {
		case http.MethodGet:
			writeJSON(w, accessControlSettingsDto(settings.Get()))
		case http.MethodPut:
			var dto shared.AccessControlSettingsDto
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&dto); err != nil {
				http.Error(w, "invalid access control settings", http.StatusBadRequest)
				return
			}
			next, err := accessControlSettingsFromDto(dto)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if persist != nil {
				if err := persist(next); err != nil {
					log.Printf("Error persisting access control settings: %v", err)
					http.Error(w, "could not persist access control settings", http.StatusInternalServerError)
					return
				}
			}
			updated := settings.Update(next)
			if broadcast != nil {
				broadcast()
			}
			writeJSON(w, accessControlSettingsDto(updated))
		default:
			w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodPut}, ", "))
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func accessControlSettingsDto(settings configuration.AccessControlSettings) shared.AccessControlSettingsDto {
	return shared.AccessControlSettingsDto{
		Proxy: accessControlConfigDto(settings.Proxy),
		WebUi: accessControlConfigDto(settings.WebUi),
	}
}

func accessControlConfigDto(config configuration.AccessControlConfig) shared.AccessControlConfigDto {
	return shared.AccessControlConfigDto{
		Mode:     string(config.Mode),
		Networks: config.Networks,
	}
}

func accessControlSettingsFromDto(dto shared.AccessControlSettingsDto) (configuration.AccessControlSettings, error) {
	proxy, err := accessControlConfigFromDto("proxy", dto.Proxy)
	if err != nil {
		return configuration.AccessControlSettings{}, err
	}
	webUi, err := accessControlConfigFromDto("web_ui", dto.WebUi)
	if err != nil {
		return configuration.AccessControlSettings{}, err
	}
	return configuration.AccessControlSettings{Proxy: proxy, WebUi: webUi}, nil
}

func accessControlConfigFromDto(name string, dto shared.AccessControlConfigDto) (configuration.AccessControlConfig, error) {
	config, err := configuration.ValidateAccessControl(configuration.AccessControlConfig{
		Mode:     configuration.AccessControlMode(dto.Mode),
		Networks: dto.Networks,
	})
	if err != nil {
		return configuration.AccessControlConfig{}, fmt.Errorf("%s: %v", name, err)
	}
	return config, nil
}

func upstreamSettingsHandler(settings *configuration.UpstreamSettingsStore, persist upstreamSettingsPersister, broadcast func()) http.HandlerFunc {
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
			if broadcast != nil {
				broadcast()
			}
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

func decryptHttpsToggleSettingsDto(config configuration.DecryptHttpsConfig) shared.DecryptHttpsToggleSettingsDto {
	return shared.DecryptHttpsToggleSettingsDto{Enabled: config.Enabled}
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

func runtimeStatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	writeJSON(w, shared.RuntimeStatsDto{MemoryBytes: stats.Sys})
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
