//go:build darwin

package certManager

import (
	"context"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func NewCertInstaller() CertInstaller {
	return macOsCertInstaller{}
}

// macOsCertInstaller is used on Mac OS
type macOsCertInstaller struct{}

// InstallCACert imports the CA certificate into the current user's login
// keychain and marks it as a trusted root. This avoids requiring administrator
// rights while still making locally intercepted HTTPS trusted for this user.
func (macOsCertInstaller) InstallCACert(caCertFile string) error {
	der, err := readCertDER(caCertFile)
	if err != nil {
		return err
	}
	if len(der) == 0 {
		return fmt.Errorf("no certificate found in %q", caCertFile)
	}

	keychain, err := loginKeychainPath()
	if err != nil {
		return err
	}

	installed, err := caCertInstalled(keychain, caCertFile, der)
	if err != nil {
		// Do not fail startup on the check. Try the import, matching the
		// Windows implementation's behavior when its pre-check fails.
		log.Printf("⚠️  Could not check the login keychain for the CA certificate: %v\n", err)
	} else if installed {
		log.Printf("🔒 CA certificate already trusted in the current user's login keychain, skipping: %s\n", caCertFile)
		return nil
	}

	if err := runSecurity("add-trusted-cert", "-r", "trustRoot", "-k", keychain, caCertFile); err != nil {
		return err
	}
	log.Printf("🔒 CA certificate installed in the current user's login keychain: %s\n", caCertFile)
	return nil
}

// InstallDomainCert imports a signed domain certificate into the current user's
// login keychain. The CA trust is what makes interception work, but keeping the
// leaf certificate visible mirrors the Windows personal-store behavior.
func (macOsCertInstaller) InstallDomainCert(domainCertFile string) error {
	// Skip installation of domain certificate under macOS
	return nil
}

// CleanupStore removes from the current user's login keychain every certificate
// this application added: the debug root CA(s), matched on marker in their
// Subject common name, and any leaf they signed, matched on the same marker in
// their Issuer common name. macOS never installs leaves (InstallDomainCert is a
// no-op), but a version that did, or a manual import, may have left some
// behind, so both are swept. Certificates the app never created carry no such
// marker and are left untouched.
//
// Deletion drops the user trust settings along with the keychain entry,
// otherwise a removed CA would keep its "always trust" entry behind and a later
// re-import would silently inherit it.
func (macOsCertInstaller) CleanupStore(marker string) (int, int, bool, error) {
	keychain, err := loginKeychainPath()
	if err != nil {
		return 0, 0, true, err
	}

	certs, err := macOSKeychainCertificates(keychain)
	if err != nil {
		return 0, 0, true, err
	}

	var rootRemoved, domainRemoved int
	var errs []error
	for _, cert := range certs {
		isRoot, isDomain := appCertificateKind(cert, marker)
		if !isRoot && !isDomain {
			continue
		}
		if err := deleteMacOSCertificate(keychain, cert.Raw); err != nil {
			errs = append(errs, err)
			continue
		}
		if isRoot {
			rootRemoved++
		} else {
			domainRemoved++
		}
	}
	return rootRemoved, domainRemoved, true, errors.Join(errs...)
}

func (macOsCertInstaller) IsCACertInstalled(caCertFile string) (bool, error) {
	der, err := readCertDER(caCertFile)
	if err != nil {
		return false, err
	}
	if len(der) == 0 {
		return false, fmt.Errorf("no certificate found in %q", caCertFile)
	}
	keychain, err := loginKeychainPath()
	if err != nil {
		return false, err
	}
	return caCertInstalled(keychain, caCertFile, der)
}

func (macOsCertInstaller) IsSupported() bool {
	return true
}

// caCertInstalled reports whether the CA is both stored in the keychain and
// marked as trusted there.
//
// Both halves are needed. A trust evaluation alone is not a usable answer:
// macOS's trustd caches successful evaluations, so `security verify-cert` keeps
// reporting a certificate as trusted long after the keychain entry backing it
// is gone. That stale "yes" made InstallCACert skip the import on every launch,
// leaving interception broken with no way to repair it from the UI. Looking the
// certificate up by fingerprint reads the keychain itself, so it is immune to
// that cache; the trust check then catches a certificate that was imported but
// never marked as a trusted root.
func caCertInstalled(keychain, certFile string, der []byte) (bool, error) {
	present, err := certInMacOSKeychain(keychain, der)
	if err != nil {
		return false, err
	}
	if !present {
		return false, nil
	}
	return macOSCertTrusted(keychain, certFile)
}

// appCertificateKind tells whether a certificate found in the keychain was
// created by this application, and if so whether it is one of our root CAs or a
// leaf one of them signed. Our roots carry the marker in their own Subject, and
// because they are self-signed they carry it in their Issuer too — so the
// Subject is tested first and wins.
func appCertificateKind(cert *x509.Certificate, marker string) (isRoot, isDomain bool) {
	if strings.Contains(cert.Subject.CommonName, marker) {
		return true, false
	}
	return false, strings.Contains(cert.Issuer.CommonName, marker)
}

func loginKeychainPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if home == "" {
		return "", fmt.Errorf("could not resolve user home directory")
	}
	return filepath.Join(home, "Library", "Keychains", "login.keychain-db"), nil
}

// macOSKeychainCertificates returns every certificate stored in the keychain.
// They are read as PEM because `security` can only filter on the subject, while
// the cleanup also has to recognize the leaves our CA issued.
func macOSKeychainCertificates(keychain string) ([]*x509.Certificate, error) {
	args := []string{"find-certificate", "-a", "-p", keychain}
	output, err := exec.Command("security", args...).CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			// A keychain holding no certificate exits non-zero with no output.
			return nil, nil
		}
		return nil, fmt.Errorf("security %s failed: %w: %s", strings.Join(args, " "), err, text)
	}
	return parsePEMCertificates(output), nil
}

// parsePEMCertificates decodes every CERTIFICATE block in data. Blocks that are
// not certificates, or that fail to parse, are skipped: one unreadable keychain
// entry must not hide all the others from the cleanup.
func parsePEMCertificates(data []byte) []*x509.Certificate {
	var certs []*x509.Certificate
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return certs
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		certs = append(certs, cert)
	}
}

// deleteMacOSCertificate removes a single certificate and its user trust
// settings. It is identified by its SHA-1 fingerprint rather than its common
// name, so the deletion can never match a neighbouring entry.
func deleteMacOSCertificate(keychain string, der []byte) error {
	return runSecurity("delete-certificate", "-Z", certFingerprint(der), "-t", keychain)
}

func certInMacOSKeychain(keychain string, der []byte) (bool, error) {
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return false, err
	}

	args := []string{"find-certificate", "-a", "-Z"}
	if cert.Subject.CommonName != "" {
		args = append(args, "-c", cert.Subject.CommonName)
	}
	args = append(args, keychain)

	output, err := exec.Command("security", args...).CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			return false, nil
		}
		return false, fmt.Errorf("security %s failed: %w: %s", strings.Join(args, " "), err, text)
	}

	return strings.Contains(strings.ToUpper(string(output)), certFingerprint(der)), nil
}

// certFingerprint returns the uppercase hex SHA-1 of a DER certificate, the form
// `security` both prints and accepts.
func certFingerprint(der []byte) string {
	sum := sha1.Sum(der)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func macOSCertTrusted(keychain, certFile string) (bool, error) {
	args := []string{"verify-cert", "-c", certFile, "-p", "ssl", "-l", "-L", "-k", keychain, "-q"}
	output, err := exec.Command("security", args...).CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			return false, nil
		}
		return false, fmt.Errorf("security %s failed: %w: %s", strings.Join(args, " "), err, text)
	}
	return true, nil
}

// securityCommandTimeout bounds a `security` invocation that changes the user's
// trust settings. Both of ours do — add-trusted-cert and delete-certificate -t —
// and each raises a system authorization dialog. That dialog is modal and waits
// indefinitely, and it does not always surface above the application window, so
// without a bound an unanswered prompt hangs the caller for good: a startup that
// never completes, or a cleanup request that never returns. The timeout leaves
// room to type a password while guaranteeing the app recovers on its own.
const securityCommandTimeout = 60 * time.Second

func runSecurity(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), securityCommandTimeout)
	defer cancel()

	output, err := exec.CommandContext(ctx, "security", args...).CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("security %s timed out after %s: the macOS authorization prompt went unanswered",
				strings.Join(args, " "), securityCommandTimeout)
		}
		text := strings.TrimSpace(string(output))
		if text == "" {
			return fmt.Errorf("security %s failed: %w", strings.Join(args, " "), err)
		}
		return fmt.Errorf("security %s failed: %w: %s", strings.Join(args, " "), err, text)
	}
	return nil
}
