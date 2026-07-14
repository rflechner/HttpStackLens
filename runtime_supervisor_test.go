package main

import (
	"httpStackLens/configuration"
	"httpStackLens/storage"
	"testing"
)

func TestRuntimeSupervisorStopsAndRestartsProxyIdempotently(t *testing.T) {
	config := configuration.DefaultAppConfig()
	config.Proxy.Port = 0
	access := configuration.NewAccessControlSettingsStore(configuration.AccessControlSettingsFromConfig(config))
	initial, err := CreateProxyServer(AppContext{port: 0}, nil, config.Proxy, access, nil, nil, storage.NewCaptureController(true))
	if err != nil {
		t.Fatalf("CreateProxyServer: %v", err)
	}
	controller := storage.NewProxyController(true)
	supervisor := &runtimeSupervisor{
		config:      newRuntimeConfigState(config),
		appContext:  AppContext{port: 0},
		proxy:       initial,
		proxyCtl:    controller,
		accessStore: access,
		captureCtl:  storage.NewCaptureController(true),
	}
	defer supervisor.closeAllProxies()

	supervisor.stopProxy()
	if controller.IsRunning() {
		t.Fatal("proxy should be stopped")
	}
	retiredCount := len(supervisor.retired)
	supervisor.stopProxy()
	if len(supervisor.retired) != retiredCount {
		t.Fatal("second stop should be idempotent")
	}

	if err := supervisor.startProxy(); err != nil {
		t.Fatalf("startProxy: %v", err)
	}
	if !controller.IsRunning() {
		t.Fatal("proxy should be running")
	}
	running := supervisor.proxy
	if err := supervisor.startProxy(); err != nil {
		t.Fatalf("second startProxy: %v", err)
	}
	if supervisor.proxy != running {
		t.Fatal("second start should not replace the running proxy")
	}
}
