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

	hub := webui.ServeWebUi(appContext.webUiPort, stopChan, config)

	caCert, caKey, err := certManager.GetHttpsDebugRootCertificates(config)
	if err != nil {
		log.Fatal(err)
	}

	// Issues and caches the per-domain certificates used to decrypt HTTPS. When
	// decrypt_https is enabled, each new domain cert is also added to the user's
	// personal store (see NewCertStoreFromConfig).
	certStore := certManager.NewCertStoreFromConfig(caCert, caKey, config)

	// Optional capture file. When decrypting, the interceptor stores clear-text
	// requests/responses; otherwise only top-level HTTP requests and CONNECTs.
	captureWriter := openCaptureWriter(config)
	if captureWriter != nil {
		defer func() { _ = captureWriter.Close() }()
	}

	// To decrypt HTTPS, the CA must be trusted by the OS so the domain
	// certificates we sign on the fly are accepted. A failure here is not fatal:
	// the user can still install the CA manually.
	if config.Capture.DecryptHttps {
		installer := certManager.NewCertInstaller()
		if !installer.IsSupported() {
			slog.Warn("Automatic certificate installation is not supported on this OS; install the CA manually",
				"caCertFile", config.CertManager.CaCertFile)
		} else if err := installer.InstallCACert(config.CertManager.CaCertFile); err != nil {
			slog.Warn("Failed to install the CA certificate in the OS trust store; install it manually",
				"caCertFile", config.CertManager.CaCertFile, "error", err)
		}

		// Insert the man-in-the-middle in front of the tunnel so CONNECT requests
		// are decrypted instead of blindly piped.
		appContext.pipeline = &middlewares.HttpsInterceptor{
			CertStore: certStore,
			Next:      appContext.pipeline,
			Capture:   captureWriter,
			Limits:    config.Capture,
		}
		slog.Info("HTTPS decryption enabled")
	}

	logger := logging.CreateWebUiEventLogger(hub)
	proxyServer := CreateProxyServer(appContext, logger, config.Proxy, config.Capture.DecryptHttps, certStore, captureWriter)

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

	w, err := storage.NewFileCaptureSessionWriter(path, config.Capture.DecryptHttps)
	if err != nil {
		slog.Warn("Could not open capture file; captures disabled", "path", path, "error", err)
		return nil
	}

	slog.Info("Capture recording enabled", "file", path, "decrypted", config.Capture.DecryptHttps)
	return w
}
