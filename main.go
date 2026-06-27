package main

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"httpStackLens/certManager"
	"httpStackLens/configuration"
	"httpStackLens/logging"
	"httpStackLens/webui"
	"log"
	"log/slog"
	"os"
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func getHttpsDebugCertificates(config configuration.AppConfig) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	if config.CertManager.CaCertFile == "" || config.CertManager.CaKeyFile == "" {
		log.Fatal("CA certificate and key files must be specified in config.yaml")
		return nil, nil, errors.New("CA certificate and key files must be specified in config.yaml")
	}

	if !fileExists(config.CertManager.CaCertFile) || !fileExists(config.CertManager.CaKeyFile) {
		err := certManager.GenerateCA(config.CertManager.CaCertFile, config.CertManager.CaKeyFile)
		if err != nil {
			log.Printf("Failed to generate CA: %v\n", err)
			return nil, nil, err
		}
	} else {
		log.Printf("🔒 CA certificate and key files already exist, skipping generation")
	}

	caCert, caKey, err := certManager.LoadCA(config.CertManager.CaCertFile, config.CertManager.CaKeyFile)
	if err != nil {
		log.Printf("Failed to load CA: %v\n", err)
		return nil, nil, err
	}
	//_, _, err = certManager.SignServerCert(caCert, caKey, []string{"example.com", "www.example.com"})
	//if err != nil {
	//	log.Printf("Failed to sign server certificate: %v\n", err)
	//	return
	//}

	return caCert, caKey, nil
}

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

	logger := logging.CreateWebUiEventLogger(hub)
	proxyServer := CreateProxyServer(appContext, logger, config.Proxy)

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
