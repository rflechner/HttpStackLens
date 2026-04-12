package main

import (
	"bufio"
	"fmt"
	"httpStackLens/configuration"
	"httpStackLens/logging"
	"httpStackLens/webui"
	"log"
	"os"
)

func main() {
	config := configuration.ReadConfiguration()

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
