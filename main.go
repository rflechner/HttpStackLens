package main

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/x509"
	"errors"
	"fmt"
	"httpStackLens/certManager"
	"httpStackLens/configuration"
	"httpStackLens/logging"
	"httpStackLens/webui"
	"log"
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

	caCert, caKey, err := getHttpsDebugCertificates(config)
	if err != nil {
		log.Printf("Failed to generate CA: %v\n", err)
		return
	}

	log.Printf("CA certificate: %s\n", caCert.Subject.CommonName)
	log.Printf("CA Key loaded: Curve=%s\n", caKey.Curve.Params().Name)

	appContext, err := CreateOsSpecificProxyPipeline(config)
	if err != nil {
		log.Printf("Failed to configure proxy pipeline: %v\n", err)
		return
	}

	stopChan := make(chan bool)

	webUiPort := 9000
	if config.WebUi.Port != 0 {
		webUiPort = config.WebUi.Port
	}

	hub := webui.ServeWebUi(webUiPort, stopChan, config)

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
