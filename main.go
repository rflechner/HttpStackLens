package main

import (
	"bufio"
	"flag"
	"fmt"
	"httpStackLens/configuration"
	"httpStackLens/logging"
	"httpStackLens/proxy/middlewares"
	"httpStackLens/storage"
	"httpStackLens/webui"
	"httpStackLens/webui/wasm/shared"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// Injected at build time via -ldflags "-X main.version=... -X main.commit=... -X main.date=...".
// These must stay package-level: ldflags -X cannot write to a local variable.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// repoURL is the GitHub repository the Web UI status bar links to for the
// currently running commit; repoSlug is the "owner/name" form used to query the
// releases API for update checks.
const (
	repoURL  = "https://github.com/rflechner/HttpStackLens"
	repoSlug = "rflechner/HttpStackLens"
)

// buildInfo assembles the version metadata for the Web UI. CommitURL is only
// set when commit holds a real hash (release builds), so dev builds don't render
// a link to a non-existent commit.
func buildInfo() shared.BuildInfoDto {
	info := shared.BuildInfoDto{Version: version, Commit: commit, Date: date}
	if commit != "" && commit != "none" {
		info.CommitURL = repoURL + "/commit/" + commit
	}
	return info
}

func main() {
	// Handled before flag.Parse (which runs later inside
	// CreateOsSpecificProxyPipeline, after -port/-webUiPort are registered), so we
	// scan os.Args directly instead of registering a flag we cannot parse yet.
	for _, arg := range os.Args[1:] {
		if arg == "-version" || arg == "--version" {
			fmt.Printf("httpStackLens %s (commit %s, built %s)\n", version, commit, date)
			return
		}
	}

	config := configuration.ReadOrCreateConfigurationIfNotExists()

	// Registered here but parsed inside CreateOsSpecificProxyPipeline (which
	// calls flag.Parse on the shared default flag set).
	verbose := flag.Bool("verbose", false, "enable verbose (debug) logging")

	appContext, err := CreateOsSpecificProxyPipeline(config)
	if err != nil {
		log.Printf("Failed to configure proxy pipeline: %v\n", err)
		return
	}

	level := logging.ParseLevel(config.Logging.Level)
	if *verbose {
		level = slog.LevelDebug
	}
	cleanup, err := logging.Setup(level, config.Logging.GetResolvedFile())
	if err != nil {
		log.Printf("Failed to set up logging: %v\n", err)
	} else {
		defer func() { _ = cleanup() }()
	}
	slog.Info("HttpStackLens starting",
		"proxyPort", appContext.port,
		"webUiPort", appContext.webUiPort,
		"level", level.String())
	logResolvedPaths(config)

	stopChan := make(chan bool)

	// Keeps the most recent request/response records in memory so the Web UI can
	// fetch their full headers and bodies on demand.
	requestStore := storage.NewRequestStore(storage.DefaultRequestStoreSize)
	// Live recording is independent from storage.enable, which only controls
	// whether the recorded traffic is also persisted to .capture files.
	captureCtl := storage.NewCaptureController(true)
	proxyCtl := storage.NewProxyController(true)
	decryptHttpsSettings := configuration.NewDecryptHttpsConfigStore(config.DecryptHttps)
	upstreamSettings := configuration.NewUpstreamSettingsStore(configuration.UpstreamSettingsFromProxyConfig(config.Proxy))
	accessControlSettings := configuration.NewAccessControlSettingsStore(configuration.AccessControlSettingsFromConfig(config))
	proxyAccess := accessControlSettings.Get().Proxy
	proxyCtl.SetAddress(fmt.Sprintf("%s:%d", proxyAccess.ListenHost(), appContext.port))
	runtimeConfig := newRuntimeConfigState(config)
	runtimeCommands := make(chan webui.RuntimeCommand, 16)
	basePipeline := appContext.pipeline
	activePipeline := middlewares.NewSwitchableMiddleware(basePipeline)
	appContext.pipeline = activePipeline

	// Runtime-switchable capture storage. When decrypting, the interceptor stores
	// clear-text requests/responses; otherwise only top-level HTTP requests and
	// CONNECTs. Persistence is independent from the live UI recording flag
	// (captureCtl): the Web UI can start/stop storage without affecting the live
	// view, and vice versa.
	storageSink := newStorageSink(config, decryptHttpsSettings)
	defer func() { _ = storageSink.Close() }()
	if config.Storage.Enable {
		if err := storageSink.Enable(); err != nil {
			slog.Warn("Could not start capture storage; storage disabled", "error", err)
		}
	}
	captureWriter := storageSink

	hub := webui.ServeWebUi(appContext.webUiPort, stopChan, webui.Dependencies{
		InitialConfig:         config,
		CurrentConfig:         runtimeConfig.Snapshot,
		DecryptHTTPSSettings:  decryptHttpsSettings,
		UpstreamSettings:      upstreamSettings,
		AccessControlSettings: accessControlSettings,
		Requests:              requestStore,
		Capture:               captureCtl,
		Storage:               storageSink,
		Proxy:                 proxyCtl,
		Commands:              runtimeCommands,
		Build:                 buildInfo(),
		GitHubRepo:            repoSlug,
		UpdateCheckEnabled:    config.Updates.CheckEnabled,
	})

	// Streams request/response events to the Web UI over SSE. Created before the
	// pipeline so the HTTPS interceptor can surface the decrypted requests and
	// responses it sees (they are otherwise only written to the capture file).
	logger := logging.CreateWebUiEventLogger(hub)

	decryptRuntime := newDecryptHttpsRuntime(config, basePipeline, activePipeline, decryptHttpsSettings, captureWriter, logger, requestStore, captureCtl, configuration.PersistDecryptHttpsEnabled)
	if err := decryptRuntime.ApplyInitial(); err != nil {
		log.Fatal(err)
	}

	proxyServer, err := CreateProxyServer(appContext, logger, config.Proxy, accessControlSettings, captureWriter, requestStore, captureCtl)
	if err != nil {
		log.Fatal(err)
	}
	proxyCtl.SetAddress(proxyServer.Address())
	supervisor := &runtimeSupervisor{
		config:        runtimeConfig,
		appContext:    appContext,
		proxy:         proxyServer,
		eventLogger:   logger,
		decrypt:       decryptRuntime,
		decryptStore:  decryptHttpsSettings,
		upstreamStore: upstreamSettings,
		accessStore:   accessControlSettings,
		capture:       captureWriter,
		storageSink:   storageSink,
		requests:      requestStore,
		captureCtl:    captureCtl,
		proxyCtl:      proxyCtl,
	}

	go proxyServer.Run()
	go supervisor.Run(runtimeCommands, stopChan)

	keyboard := bufio.NewReader(os.Stdin)

	go func() {
		fmt.Println("Type 'exit' to quit")
		for {
			line, _, _ := keyboard.ReadLine()
			if string(line) == "exit" {
				close(stopChan)
			}
		}
	}()

	select {
	case <-stopChan:
		supervisor.closeAllProxies()
	}
}

// logResolvedPaths reports the on-disk locations the app will actually use, once
// resolved relative to the executable (or kept as-is when absolute). Relative
// paths in config.yaml are resolved against the binary, not the working
// directory, so this makes the effective locations explicit at startup.
func logResolvedPaths(config configuration.AppConfig) {
	certManager := config.DecryptHttps.CertManager
	slog.Info("Resolved configuration paths",
		"config", configuration.ResolveConfigPath(),
		"logFile", config.Logging.GetResolvedFile(),
		"storageEnabled", config.Storage.Enable,
		"storageFolder", config.Storage.GetResolvedFolder(),
		"caCertFile", certManager.GetResolvedCaCertFile(),
		"caKeyFile", certManager.GetResolvedCaKeyFile(),
		"domainCertsFolder", certManager.GetResolvedDomainCertsFolder())
}

// newStorageSink builds the runtime-switchable capture destination. Each time
// storage is switched on (at startup when storage.enable is true, or later from
// the Web UI) it opens a fresh timestamped .capture file in the configured
// folder — resolved against the executable, or used as-is when absolute. The
// HTTPS-decrypted flag stamped in each file reflects the decrypt state at the
// moment storage is turned on. A failure to open a file surfaces to whoever
// toggled storage, without aborting the proxy.
func newStorageSink(config configuration.AppConfig, decrypt *configuration.DecryptHttpsConfigStore) *storage.StorageSink {
	folder := config.Storage.GetResolvedFolder()
	return storage.NewStorageSink(func() (storage.CaptureSessionWriter, error) {
		if err := os.MkdirAll(folder, 0o755); err != nil {
			return nil, fmt.Errorf("could not create capture folder %q: %w", folder, err)
		}

		name := fmt.Sprintf("capture-%s.capture", time.Now().Format("20060102-150405"))
		path := filepath.Join(folder, name)

		decrypted := config.DecryptHttps.Enabled
		if decrypt != nil {
			decrypted = decrypt.Get().Enabled
		}

		w, err := storage.NewFileCaptureSessionWriter(path, decrypted)
		if err != nil {
			return nil, fmt.Errorf("could not open capture file %q: %w", path, err)
		}

		slog.Info("Capture storage started", "file", path, "decrypted", decrypted)
		return w, nil
	})
}
