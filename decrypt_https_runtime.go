package main

import (
	"fmt"
	"httpStackLens/certManager"
	"httpStackLens/configuration"
	"httpStackLens/proxy/middlewares"
	"httpStackLens/storage"
	"log/slog"
	"sync"
)

type decryptHttpsRuntime struct {
	mu        sync.Mutex
	config    configuration.AppConfig
	base      middlewares.Middleware
	active    *middlewares.SwitchableMiddleware
	settings  *configuration.DecryptHttpsConfigStore
	capture   storage.CaptureSessionWriter
	events    middlewares.EventSink
	store     *storage.RequestStore
	captureCt *storage.CaptureController
	persist   func(bool) error
}

func newDecryptHttpsRuntime(config configuration.AppConfig, base middlewares.Middleware, active *middlewares.SwitchableMiddleware, settings *configuration.DecryptHttpsConfigStore, capture storage.CaptureSessionWriter, events middlewares.EventSink, store *storage.RequestStore, captureCtl *storage.CaptureController, persist func(bool) error) *decryptHttpsRuntime {
	return &decryptHttpsRuntime{
		config:    config,
		base:      base,
		active:    active,
		settings:  settings,
		capture:   capture,
		events:    events,
		store:     store,
		captureCt: captureCtl,
		persist:   persist,
	}
}

func (r *decryptHttpsRuntime) ApplyInitial() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.settings.Get().Enabled {
		r.active.SetDecrypting(r.base, false)
		return nil
	}
	interceptor, err := r.newInterceptor()
	if err != nil {
		return err
	}
	r.active.SetDecrypting(interceptor, true)
	slog.Info("HTTPS decryption enabled")
	return nil
}

func (r *decryptHttpsRuntime) SetEnabled(enabled bool) (configuration.DecryptHttpsConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current := r.settings.Get()
	if current.Enabled == enabled {
		return current, nil
	}

	var next middlewares.Middleware
	if enabled {
		interceptor, err := r.newInterceptor()
		if err != nil {
			return current, err
		}
		next = interceptor
	} else {
		next = r.base
	}

	if r.persist != nil {
		if err := r.persist(enabled); err != nil {
			return current, fmt.Errorf("could not persist HTTPS decryption setting: %w", err)
		}
	}

	updated := r.settings.UpdateEnabled(enabled)
	r.active.SetDecrypting(next, enabled)
	if enabled {
		slog.Info("HTTPS decryption enabled")
	} else {
		slog.Info("HTTPS decryption disabled")
	}
	return updated, nil
}

func (r *decryptHttpsRuntime) newInterceptor() (*middlewares.HttpsInterceptor, error) {
	caCert, caKey, err := certManager.GetHttpsDebugRootCertificates(r.config)
	if err != nil {
		return nil, err
	}

	certStore := certManager.NewCertStoreFromConfig(caCert, caKey, r.config)
	installer := certManager.NewCertInstaller()
	if !installer.IsSupported() {
		slog.Warn("Automatic certificate installation is not supported on this OS; install the CA manually",
			"caCertFile", r.config.DecryptHttps.CertManager.CaCertFile)
	} else if err := installer.InstallCACert(r.config.DecryptHttps.CertManager.CaCertFile); err != nil {
		slog.Warn("Failed to install the CA certificate in the OS trust store; install it manually",
			"caCertFile", r.config.DecryptHttps.CertManager.CaCertFile, "error", err)
	}

	return &middlewares.HttpsInterceptor{
		CertStore:  certStore,
		Next:       r.base,
		Capture:    r.capture,
		Limits:     r.settings,
		Events:     r.events,
		Store:      r.store,
		CaptureCtl: r.captureCt,
	}, nil
}
