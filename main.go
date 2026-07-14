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
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

func main() {
	config := configuration.ReadConfiguration()

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
	cleanup, err := logging.Setup(level, config.Logging.File)
	if err != nil {
		log.Printf("Failed to set up logging: %v\n", err)
	} else {
		defer func() { _ = cleanup() }()
	}
	slog.Info("HttpStackLens starting",
		"proxyPort", appContext.port,
		"webUiPort", appContext.webUiPort,
		"level", level.String())

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
	runtimeConfig := newRuntimeConfigState(config)
	runtimeCommands := make(chan webui.RuntimeCommand, 16)
	basePipeline := appContext.pipeline
	activePipeline := middlewares.NewSwitchableMiddleware(basePipeline)
	appContext.pipeline = activePipeline

	// Optional capture file. When decrypting, the interceptor stores clear-text
	// requests/responses; otherwise only top-level HTTP requests and CONNECTs.
	captureWriter := openCaptureWriter(config)
	if captureWriter != nil {
		defer func() { _ = captureWriter.Close() }()
	}

	hub := webui.ServeWebUi(appContext.webUiPort, stopChan, webui.Dependencies{
		InitialConfig:         config,
		CurrentConfig:         runtimeConfig.Snapshot,
		DecryptHTTPSSettings:  decryptHttpsSettings,
		UpstreamSettings:      upstreamSettings,
		AccessControlSettings: accessControlSettings,
		Requests:              requestStore,
		Capture:               captureCtl,
		Proxy:                 proxyCtl,
		Commands:              runtimeCommands,
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

// openCaptureWriter creates a timestamped .capture file in the configured folder
// when storage is enabled. A relative folder is resolved against the working
// directory; an absolute folder is used as-is. Any failure disables capturing
// without aborting startup.
func openCaptureWriter(config configuration.AppConfig) storage.CaptureSessionWriter {
	if !config.Storage.Enable {
		return nil
	}

	folder := config.Storage.Folder
	if folder == "" {
		folder = "captures"
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		slog.Warn("Could not create capture folder; captures disabled", "folder", folder, "error", err)
		return nil
	}

	name := fmt.Sprintf("capture-%s.capture", time.Now().Format("20060102-150405"))
	path := filepath.Join(folder, name)

	w, err := storage.NewFileCaptureSessionWriter(path, config.DecryptHttps.Enabled)
	if err != nil {
		slog.Warn("Could not open capture file; captures disabled", "path", path, "error", err)
		return nil
	}

	slog.Info("Capture recording enabled", "file", path, "decrypted", config.DecryptHttps.Enabled)
	return w
}
