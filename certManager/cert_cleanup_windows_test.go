//go:build windows

package certManager

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"strings"
	"testing"
	"time"
)

// testStoreName is an application-private per-user certificate store. Using a
// dedicated name keeps the test fully isolated from the real ROOT/MY trust
// stores, so it can add and delete certificates without any side effect on the
// machine's actual trust configuration.
const testStoreName = "HttpStackLensCleanupTest"

// makeSelfSigned returns the DER bytes of a throwaway self-signed certificate
// with the given common name.
func makeSelfSigned(t *testing.T, commonName string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("serial: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	return der
}

// TestCleanupStoreOnlyRemovesMarkedCertificates is the core safety test for the
// user's concern: cleanup must delete only the certificates this application
// created (identified by the CA common-name marker) and leave unrelated ones
// alone. It runs against an isolated per-user store, never ROOT/MY.
func TestCleanupStoreOnlyRemovesMarkedCertificates(t *testing.T) {
	ours := makeSelfSigned(t, "somehost"+caCommonNameSuffix) // carries the marker
	unrelated := makeSelfSigned(t, "Some Unrelated Corp Root CA")

	if err := addEncodedCertToStore(testStoreName, ours); err != nil {
		t.Fatalf("add ours: %v", err)
	}
	if err := addEncodedCertToStore(testStoreName, unrelated); err != nil {
		t.Fatalf("add unrelated: %v", err)
	}
	// Make sure the store is clean at the end regardless of assertions.
	t.Cleanup(func() {
		store, err := openCurrentUserStore(testStoreName)
		if err != nil {
			return
		}
		defer procCertCloseStore.Call(store, 0)
		_, _ = deleteCertFromStore(store, ours)
		_, _ = deleteCertFromStore(store, unrelated)
	})

	removed, err := cleanupStore(testStoreName, func(c *x509.Certificate) bool {
		return strings.Contains(c.Subject.CommonName, caCommonNameMarker)
	})
	if err != nil {
		t.Fatalf("cleanupStore: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1 (only the marked cert)", removed)
	}

	if present, _ := certInStore(testStoreName, ours); present {
		t.Error("marked certificate still present after cleanup")
	}
	if present, _ := certInStore(testStoreName, unrelated); !present {
		t.Error("unrelated certificate was removed — differentiation failed")
	}
}
