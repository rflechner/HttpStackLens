package main

import (
	"bufio"
	"flag"
	"fmt"
	"httpStackLens/certManager"
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

	hub := webui.ServeWebUi(appContext.webUiPort, stopChan, config, requestStore)

	var certStore *certManager.CertStore

	// Optional capture file. When decrypting, the interceptor stores clear-text
	// requests/responses; otherwise only top-level HTTP requests and CONNECTs.
	captureWriter := openCaptureWriter(config)
	if captureWriter != nil {
		defer func() { _ = captureWriter.Close() }()
	}

	// Streams request/response events to the Web UI over SSE. Created before the
	// pipeline so the HTTPS interceptor can surface the decrypted requests and
	// responses it sees (they are otherwise only written to the capture file).
	logger := logging.CreateWebUiEventLogger(hub)

	// To decrypt HTTPS, the CA must be trusted by the OS so the domain
	// certificates we sign on the fly are accepted. A failure here is not fatal:
	// the user can still install the CA manually.
	if config.DecryptHttps.Enabled {
		caCert, caKey, err := certManager.GetHttpsDebugRootCertificates(config)
		if err != nil {
			log.Fatal(err)
		}

		// Issues and caches the per-domain certificates used to decrypt HTTPS.
		// Each new domain cert is also added to the user's personal store (see
		// NewCertStoreFromConfig).
		certStore = certManager.NewCertStoreFromConfig(caCert, caKey, config)

		installer := certManager.NewCertInstaller()
		if !installer.IsSupported() {
			slog.Warn("Automatic certificate installation is not supported on this OS; install the CA manually",
				"caCertFile", config.DecryptHttps.CertManager.CaCertFile)
		} else if err := installer.InstallCACert(config.DecryptHttps.CertManager.CaCertFile); err != nil {
			slog.Warn("Failed to install the CA certificate in the OS trust store; install it manually",
				"caCertFile", config.DecryptHttps.CertManager.CaCertFile, "error", err)
		}

		// Insert the man-in-the-middle in front of the tunnel so CONNECT requests
		// are decrypted instead of blindly piped.
		appContext.pipeline = &middlewares.HttpsInterceptor{
			CertStore: certStore,
			Next:      appContext.pipeline,
			Capture:   captureWriter,
			Limits:    config.DecryptHttps,
			Events:    logger,
			Store:     requestStore,
		}
		slog.Info("HTTPS decryption enabled")
	}

	proxyServer := CreateProxyServer(appContext, logger, config.Proxy, config.DecryptHttps.Enabled, certStore, captureWriter, requestStore)

	go proxyServer.Run()

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
		proxyServer.Close()
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
