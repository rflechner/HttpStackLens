package main

import (
	"bufio"
	"flag"
	"fmt"
	"httpStackLens/certManager"
	"httpStackLens/configuration"
	"httpStackLens/logging"
	"httpStackLens/proxy/middlewares"
	"httpStackLens/webui"
	"log"
	"log/slog"
	"os"
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

	// To decrypt HTTPS, the CA must be trusted by the OS so the domain
	// certificates we sign on the fly are accepted. A failure here is not fatal:
	// the user can still install the CA manually.
	if config.Proxy.DecryptHttps {
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
		}
		slog.Info("HTTPS decryption enabled")
	}

	logger := logging.CreateWebUiEventLogger(hub)
	proxyServer := CreateProxyServer(appContext, logger, config.Proxy, certStore)

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
