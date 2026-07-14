package main

import (
	"fmt"
	"httpStackLens/configuration"
	"httpStackLens/storage"
	"httpStackLens/webui"
	"sync"
)

type runtimeConfigState struct {
	mu     sync.RWMutex
	config configuration.AppConfig
}

func newRuntimeConfigState(config configuration.AppConfig) *runtimeConfigState {
	return &runtimeConfigState{config: config}
}

func (s *runtimeConfigState) Snapshot() configuration.AppConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	config := s.config
	config.Proxy.NoProxy = append([]string(nil), config.Proxy.NoProxy...)
	config.Proxy.AccessControl.Networks = append([]string(nil), config.Proxy.AccessControl.Networks...)
	config.WebUi.AccessControl.Networks = append([]string(nil), config.WebUi.AccessControl.Networks...)
	config.DecryptHttps = configuration.NewDecryptHttpsConfigStore(config.DecryptHttps).Get()
	return config
}

func (s *runtimeConfigState) Update(update func(*configuration.AppConfig)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	update(&s.config)
}

type runtimeSupervisor struct {
	config        *runtimeConfigState
	appContext    AppContext
	proxy         *ProxyServer
	retired       []*ProxyServer
	proxyCtl      *storage.ProxyController
	eventLogger   ProxyEventLogger
	decrypt       *decryptHttpsRuntime
	decryptStore  *configuration.DecryptHttpsConfigStore
	upstreamStore *configuration.UpstreamSettingsStore
	accessStore   *configuration.AccessControlSettingsStore
	capture       storage.CaptureSessionWriter
	requests      *storage.RequestStore
	captureCtl    *storage.CaptureController
}

func (s *runtimeSupervisor) Run(commands <-chan webui.RuntimeCommand, stop <-chan bool) {
	for {
		select {
		case command := <-commands:
			result := s.apply(command)
			command.Reply <- result
		case <-stop:
			return
		}
	}
}

func (s *runtimeSupervisor) apply(command webui.RuntimeCommand) webui.RuntimeCommandResult {
	var result webui.RuntimeCommandResult
	switch command.Kind {
	case webui.SetStorageEnabled:
		result.Err = configuration.PersistStorageEnabled(command.Enabled)
		if result.Err == nil {
			s.config.Update(func(config *configuration.AppConfig) { config.Storage.Enable = command.Enabled })
		}

	case webui.SetBodyCapture:
		result.Err = configuration.PersistDecryptHttpsCaptureRules(command.DecryptHTTPS)
		if result.Err == nil {
			updated := s.decryptStore.UpdateCaptureRules(command.DecryptHTTPS.DefaultMaxBytes, command.DecryptHTTPS.MimeTypes)
			s.config.Update(func(config *configuration.AppConfig) { config.DecryptHttps = updated })
		}

	case webui.SetDecryptHTTPS:
		result.DecryptHTTPS, result.Err = s.decrypt.SetEnabled(command.Enabled)
		if result.Err == nil {
			updated := result.DecryptHTTPS
			s.config.Update(func(config *configuration.AppConfig) { config.DecryptHttps = updated })
		}

	case webui.SetUpstream:
		result.Err = s.setUpstream(command.Upstream)

	case webui.SetAccessControl:
		result.Err = s.setAccessControl(command.AccessControl)

	case webui.StartProxy:
		result.Err = s.startProxy()
		result.ProxyRunning = s.proxyCtl.IsRunning()

	case webui.StopProxy:
		s.stopProxy()
		result.ProxyRunning = s.proxyCtl.IsRunning()

	default:
		result.Err = fmt.Errorf("unknown runtime command %d", command.Kind)
	}
	return result
}

func (s *runtimeSupervisor) startProxy() error {
	if s.proxyCtl.IsRunning() {
		return nil
	}
	proxy, err := CreateProxyServer(s.appContext, s.eventLogger, s.config.Snapshot().Proxy, s.accessStore, s.capture, s.requests, s.captureCtl)
	if err != nil {
		return err
	}
	s.proxy = proxy
	s.proxyCtl.SetRunning(true)
	go proxy.Run()
	return nil
}

func (s *runtimeSupervisor) stopProxy() {
	if !s.proxyCtl.IsRunning() {
		return
	}
	s.proxy.StopAccepting()
	s.retired = append(s.retired, s.proxy)
	s.proxyCtl.SetRunning(false)
}

func (s *runtimeSupervisor) closeAllProxies() {
	if s.proxy != nil {
		s.proxy.Close()
	}
	for _, proxy := range s.retired {
		if proxy != nil && proxy != s.proxy {
			proxy.Close()
		}
	}
	s.proxyCtl.SetRunning(false)
}

func (s *runtimeSupervisor) setUpstream(settings configuration.UpstreamSettings) error {
	current := s.config.Snapshot()
	next := current
	next.Proxy.OutputProxyUri = settings.OutputProxyUri
	next.Proxy.NoProxy = append([]string(nil), settings.NoProxy...)
	next.Proxy.AddWindowsAuthenticationToOutputProxy = settings.AddWindowsAuthentication

	base, err := RebuildOsSpecificProxyPipeline(next.Proxy)
	if err != nil {
		return err
	}
	if err := s.decrypt.ReplaceBase(base); err != nil {
		return err
	}
	if err := configuration.PersistUpstreamSettings(settings); err != nil {
		oldBase, rebuildErr := RebuildOsSpecificProxyPipeline(current.Proxy)
		if rebuildErr == nil {
			_ = s.decrypt.ReplaceBase(oldBase)
		}
		return err
	}

	s.upstreamStore.Update(settings)
	s.config.Update(func(config *configuration.AppConfig) { config.Proxy = next.Proxy })
	return nil
}

func (s *runtimeSupervisor) setAccessControl(settings configuration.AccessControlSettings) error {
	old := s.accessStore.Get()
	restart := s.proxyCtl.IsRunning() && old.Proxy.ListenHost() != settings.Proxy.ListenHost()

	if err := configuration.PersistAccessControlSettings(settings); err != nil {
		return err
	}

	if restart {
		s.proxy.StopAccepting()
		s.retired = append(s.retired, s.proxy)
		s.accessStore.Update(settings)
		proxy, err := CreateProxyServer(s.appContext, s.eventLogger, s.config.Snapshot().Proxy, s.accessStore, s.capture, s.requests, s.captureCtl)
		if err != nil {
			s.accessStore.Update(old)
			persistErr := configuration.PersistAccessControlSettings(old)
			rollback, rollbackErr := CreateProxyServer(s.appContext, s.eventLogger, s.config.Snapshot().Proxy, s.accessStore, s.capture, s.requests, s.captureCtl)
			if rollbackErr == nil {
				s.proxy = rollback
				go rollback.Run()
			}
			if persistErr != nil {
				return fmt.Errorf("%w (also failed to restore persisted access control: %v)", err, persistErr)
			}
			return err
		}
		s.proxy = proxy
		go proxy.Run()
	} else {
		s.accessStore.Update(settings)
	}
	s.config.Update(func(config *configuration.AppConfig) {
		config.Proxy.AccessControl = settings.Proxy
		config.WebUi.AccessControl = settings.WebUi
	})
	return nil
}
