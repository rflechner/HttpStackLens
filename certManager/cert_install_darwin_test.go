//go:build darwin

package certManager

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"path/filepath"
	"testing"
	"time"
)

// testCA generates a debug root CA with the real GenerateCA, so the tests run
// against the exact subject the cleanup has to recognize.
func testCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	dir := t.TempDir()
	certFile := filepath.Join(dir, "ca.crt")
	keyFile := filepath.Join(dir, "ca.key")
	if err := GenerateCA(certFile, keyFile); err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	cert, key, err := LoadCA(certFile, keyFile)
	if err != nil {
		t.Fatalf("LoadCA: %v", err)
	}
	return cert, key
}

// unrelatedCert is a self-signed certificate that this application never
// created: neither its subject nor its issuer carries the marker.
func unrelatedCert(t *testing.T) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(42),
		Subject:               pkix.Name{CommonName: "Some Other Root CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return cert
}

func TestAppCertificateKindClassifiesRootLeafAndStranger(t *testing.T) {
	ca, caKey := testCA(t)

	leafDER, _, err := signServerCert(ca, caKey, []string{"example.com"})
	if err != nil {
		t.Fatalf("signServerCert: %v", err)
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}

	cases := []struct {
		name           string
		cert           *x509.Certificate
		wantRoot, want bool
	}{
		{"root CA", ca, true, false},
		{"signed leaf", leaf, false, true},
		{"unrelated certificate", unrelatedCert(t), false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isRoot, isDomain := appCertificateKind(tc.cert, caCommonNameMarker)
			if isRoot != tc.wantRoot || isDomain != tc.want {
				t.Errorf("appCertificateKind() = (%v, %v), want (%v, %v)",
					isRoot, isDomain, tc.wantRoot, tc.want)
			}
		})
	}
}

func TestParsePEMCertificatesSkipsUnreadableBlocks(t *testing.T) {
	ca, _ := testCA(t)
	other := unrelatedCert(t)

	var data []byte
	// A block that is not a certificate at all.
	data = append(data, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte("not a cert")})...)
	data = append(data, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})...)
	// A CERTIFICATE block whose payload cannot be parsed.
	data = append(data, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte{0x01, 0x02}})...)
	data = append(data, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: other.Raw})...)

	certs := parsePEMCertificates(data)
	if len(certs) != 2 {
		t.Fatalf("parsePEMCertificates() returned %d certificates, want 2", len(certs))
	}
	if certs[0].Subject.CommonName != ca.Subject.CommonName {
		t.Errorf("first certificate = %q, want %q", certs[0].Subject.CommonName, ca.Subject.CommonName)
	}
	if certs[1].Subject.CommonName != other.Subject.CommonName {
		t.Errorf("second certificate = %q, want %q", certs[1].Subject.CommonName, other.Subject.CommonName)
	}
}

func TestParsePEMCertificatesOnEmptyInput(t *testing.T) {
	if certs := parsePEMCertificates(nil); len(certs) != 0 {
		t.Errorf("parsePEMCertificates(nil) = %d certificates, want 0", len(certs))
	}
}

func TestCertFingerprintMatchesSecurityOutputFormat(t *testing.T) {
	ca, _ := testCA(t)
	got := certFingerprint(ca.Raw)
	if len(got) != 40 {
		t.Fatalf("certFingerprint() = %q (%d chars), want 40 hex chars", got, len(got))
	}
	for _, r := range got {
		if !((r >= '0' && r <= '9') || (r >= 'A' && r <= 'F')) {
			t.Fatalf("certFingerprint() = %q, want uppercase hex only", got)
		}
	}
}
