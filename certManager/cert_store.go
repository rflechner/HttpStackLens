package certManager

import (
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"httpStackLens/configuration"
	"log"
	"path/filepath"
	"sync"
)

// CertStore hands out per-domain TLS certificates for HTTPS interception.
// Certificates are signed once by the local CA, then reused: first from an
// in-memory cache, then from the on-disk folder, and only generated (and
// persisted) when neither holds them yet. It is safe for concurrent use.
type CertStore struct {
	ca          *x509.Certificate
	caKey       *ecdsa.PrivateKey
	certsFolder string
	installer   CertInstaller // may be nil to skip OS trust-store installation

	mu    sync.RWMutex
	cache map[string]*tls.Certificate
}

// NewCertStore builds a store backed by the given CA and persistence folder.
//
// If installer is non-nil and supported on this OS, each newly issued domain
// certificate is also added to the user's personal store. Pass nil to only
// generate and cache certificates without touching the OS trust store.
func NewCertStore(ca *x509.Certificate, caKey *ecdsa.PrivateKey, certsFolder string, installer CertInstaller) *CertStore {
	return &CertStore{
		ca:          ca,
		caKey:       caKey,
		certsFolder: certsFolder,
		installer:   installer,
		cache:       make(map[string]*tls.Certificate),
	}
}

// NewCertStoreFromConfig builds a store from the application configuration.
//
// Per-domain certificates are installed into the user's personal store only
// when capture.decrypt_https is true; otherwise no OS trust-store change is made.
func NewCertStoreFromConfig(ca *x509.Certificate, caKey *ecdsa.PrivateKey, config configuration.AppConfig) *CertStore {
	var installer CertInstaller
	if config.Capture.DecryptHttps {
		installer = NewCertInstaller()
	}
	return NewCertStore(ca, caKey, config.CertManager.DomainCertsFolder, installer)
}

// GetCertificate returns the TLS certificate for domain, creating it if needed.
func (s *CertStore) GetCertificate(domain string) (*tls.Certificate, error) {
	// Fast path: already in memory.
	s.mu.RLock()
	if cert, ok := s.cache[domain]; ok {
		s.mu.RUnlock()
		return cert, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Another goroutine may have filled it while we waited for the lock.
	if cert, ok := s.cache[domain]; ok {
		return cert, nil
	}

	cert, err := s.loadOrCreate(domain)
	if err != nil {
		return nil, err
	}
	s.cache[domain] = cert
	return cert, nil
}

// loadOrCreate returns the certificate from disk if it exists, otherwise signs
// a new one (which CreateDomainCert also persists to certsFolder).
func (s *CertStore) loadOrCreate(domain string) (*tls.Certificate, error) {
	base := filepath.Join(s.certsFolder, sanitizeDomain(domain))
	certPath := base + ".crt"
	keyPath := base + ".key"

	if fileExists(certPath) && fileExists(keyPath) {
		tlsCert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err == nil {
			s.installDomainCert(domain, certPath)
			return &tlsCert, nil
		}
		// Corrupted/unreadable pair: log and fall through to regenerate.
		log.Printf("⚠️  Failed to load cached certificate for %s, regenerating: %v\n", domain, err)
	}

	certDER, key, err := CreateDomainCert(s.ca, s.caKey, domain, s.certsFolder)
	if err != nil {
		return nil, err
	}

	leaf, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, err
	}

	s.installDomainCert(domain, certPath)

	return &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
		Leaf:        leaf,
	}, nil
}

// installDomainCert adds the freshly issued/loaded certificate to the user's
// personal store when an installer is configured. Failures are logged but not
// fatal: interception still works thanks to the trusted CA, so a missing
// personal-store entry must not break the request.
func (s *CertStore) installDomainCert(domain, certPath string) {
	if s.installer == nil || !s.installer.IsSupported() {
		return
	}
	if err := s.installer.InstallDomainCert(certPath); err != nil {
		log.Printf("⚠️  Failed to install domain certificate for %s: %v\n", domain, err)
	}
}
