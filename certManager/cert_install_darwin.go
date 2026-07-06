//go:build darwin

package certManager

import (
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	trusted, err := macOSCertTrusted(keychain, caCertFile)
	if err != nil {
		// Do not fail startup on the trust check. Try the import, matching the
		// Windows implementation's behavior when its pre-check fails.
		log.Printf("⚠️  Could not check the login keychain trust settings for the CA certificate: %v\n", err)
	} else if trusted {
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

func (macOsCertInstaller) IsCACertInstalled(caCertFile string) (bool, error) {
	keychain, err := loginKeychainPath()
	if err != nil {
		return false, err
	}
	return macOSCertTrusted(keychain, caCertFile)
}

func (macOsCertInstaller) IsSupported() bool {
	return true
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

	sum := sha1.Sum(der)
	fingerprint := strings.ToUpper(hex.EncodeToString(sum[:]))
	return strings.Contains(strings.ToUpper(string(output)), fingerprint), nil
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

func runSecurity(args ...string) error {
	output, err := exec.Command("security", args...).CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			return fmt.Errorf("security %s failed: %w", strings.Join(args, " "), err)
		}
		return fmt.Errorf("security %s failed: %w: %s", strings.Join(args, " "), err, text)
	}
	return nil
}
