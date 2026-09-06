package certManager

import (
	"errors"
	"httpStackLens/configuration"
	"os"
	"path/filepath"
	"testing"
)

func TestSafeToRemoveRejectsDangerousPaths(t *testing.T) {
	dangerous := []string{"", ".", string(filepath.Separator)}
	for _, p := range dangerous {
		if safeToRemove(p) {
			t.Errorf("safeToRemove(%q) = true, want false", p)
		}
	}

	safe := []string{"certificates/domains", filepath.Join("a", "b", "c")}
	for _, p := range safe {
		if !safeToRemove(p) {
			t.Errorf("safeToRemove(%q) = false, want true", p)
		}
	}
}

func TestCleanupAppCertificatesRemovesFilesAndFolder(t *testing.T) {
	dir := t.TempDir()
	caCert := filepath.Join(dir, "debug-https-ca.crt")
	caKey := filepath.Join(dir, "debug-https-ca.key")
	domains := filepath.Join(dir, "domains")

	if err := GenerateCA(caCert, caKey); err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	if err := os.MkdirAll(domains, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(domains, "example.com.crt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	certConfig := configuration.CertManagerConfig{
		CaCertFile:        caCert,
		CaKeyFile:         caKey,
		DomainCertsFolder: domains,
	}

	report, err := CleanupAppCertificates(certConfig, noopInstaller{})
	if err != nil {
		t.Fatalf("CleanupAppCertificates: %v", err)
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", report.Warnings)
	}
	if report.StoreCleanupSupported {
		t.Error("StoreCleanupSupported = true for noop installer, want false")
	}
	if !report.DomainFolderRemoved {
		t.Error("DomainFolderRemoved = false, want true")
	}
	if len(report.RemovedFiles) != 2 {
		t.Errorf("RemovedFiles = %v, want the 2 CA files", report.RemovedFiles)
	}

	for _, p := range []string{caCert, caKey, domains} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s still exists after cleanup (err=%v)", p, err)
		}
	}
}

func TestCleanupAppCertificatesToleratesMissingArtifacts(t *testing.T) {
	dir := t.TempDir()
	certConfig := configuration.CertManagerConfig{
		CaCertFile:        filepath.Join(dir, "missing.crt"),
		CaKeyFile:         filepath.Join(dir, "missing.key"),
		DomainCertsFolder: filepath.Join(dir, "missing-domains"),
	}

	report, err := CleanupAppCertificates(certConfig, noopInstaller{})
	if err != nil {
		t.Fatalf("CleanupAppCertificates: %v", err)
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("missing artifacts should not warn, got: %v", report.Warnings)
	}
	if report.DomainFolderRemoved {
		t.Error("DomainFolderRemoved = true for a missing folder, want false")
	}
	if len(report.RemovedFiles) != 0 {
		t.Errorf("RemovedFiles = %v, want none", report.RemovedFiles)
	}
}

// failingStoreInstaller stands in for a trust-store cleanup that cannot
// complete: on macOS the removal needs an interactive authorization the user can
// cancel, or leave unanswered until it times out.
type failingStoreInstaller struct{ noopInstaller }

func (failingStoreInstaller) IsSupported() bool { return true }

func (failingStoreInstaller) CleanupStore(string) (int, int, bool, error) {
	return 0, 0, true, errors.New("the authorization was canceled by the user")
}

// TestCleanupAppCertificatesCleansDiskDespiteStoreFailure pins the ordering: the
// on-disk cleanup needs no authorization, so it must already be done by the time
// the trust store is touched. Otherwise a single unanswered system dialog leaves
// the user with nothing removed at all.
func TestCleanupAppCertificatesCleansDiskDespiteStoreFailure(t *testing.T) {
	dir := t.TempDir()
	caCert := filepath.Join(dir, "debug-https-ca.crt")
	caKey := filepath.Join(dir, "debug-https-ca.key")
	domains := filepath.Join(dir, "domains")

	if err := GenerateCA(caCert, caKey); err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	if err := os.MkdirAll(domains, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	report, err := CleanupAppCertificates(configuration.CertManagerConfig{
		CaCertFile:        caCert,
		CaKeyFile:         caKey,
		DomainCertsFolder: domains,
	}, failingStoreInstaller{})
	if err != nil {
		t.Fatalf("CleanupAppCertificates: %v", err)
	}

	if len(report.RemovedFiles) != 2 {
		t.Errorf("RemovedFiles = %v, want the CA certificate and key", report.RemovedFiles)
	}
	if !report.DomainFolderRemoved {
		t.Error("DomainFolderRemoved = false, want true")
	}
	for _, path := range []string{caCert, caKey, domains} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Errorf("%s still exists after cleanup", path)
		}
	}
	if len(report.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want the trust store failure to be reported", report.Warnings)
	}
}
