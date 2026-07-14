package certManager

import (
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
