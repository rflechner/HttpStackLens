package certManager

import (
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
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

	mu    sync.RWMutex
	cache map[string]*tls.Certificate
}

// NewCertStore builds a store backed by the given CA and persistence folder.
func NewCertStore(ca *x509.Certificate, caKey *ecdsa.PrivateKey, certsFolder string) *CertStore {
	return &CertStore{
		ca:          ca,
		caKey:       caKey,
		certsFolder: certsFolder,
		cache:       make(map[string]*tls.Certificate),
	}
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

	return &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
		Leaf:        leaf,
	}, nil
}
