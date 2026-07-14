package certManager

import (
	"errors"
	"fmt"
	"httpStackLens/configuration"
	"os"
	"path/filepath"
	"strings"
)

// caCommonNameMarker is the fixed, distinctive fragment embedded in every debug
// CA's subject common name by GenerateCA (see caCommonNameSuffix). It is the
// signature cleanup relies on to tell this application's certificates apart from
// unrelated ones in the OS trust store: our root CAs carry it in their Subject,
// and every per-domain leaf we sign carries it in its Issuer. No third-party
// certificate is expected to contain this string, so matching on it never
// touches certificates the app did not create.
const caCommonNameMarker = "My Local CA for debugging HTTPS"

// caCommonNameSuffix is appended to the machine hostname to form the CA's common
// name. Keeping it next to caCommonNameMarker ensures generation and cleanup
// stay in sync.
const caCommonNameSuffix = " - " + caCommonNameMarker

// CleanupReport summarizes what CleanupAppCertificates removed.
type CleanupReport struct {
	// RootCertsRemoved / DomainCertsRemoved count entries deleted from the OS
	// trust store (the current user's Root and personal stores on Windows).
	RootCertsRemoved   int
	DomainCertsRemoved int
	// StoreCleanupSupported is false when automatic OS trust-store cleanup is not
	// implemented for the current operating system; the on-disk cleanup below
	// still runs.
	StoreCleanupSupported bool
	// RemovedFiles lists the CA certificate/key files deleted from disk.
	RemovedFiles []string
	// DomainFolderRemoved is true when the per-domain certificates folder was
	// deleted.
	DomainFolderRemoved bool
	// Warnings collects non-fatal problems (e.g. a single file that could not be
	// deleted) so the caller can surface them without failing the whole cleanup.
	Warnings []string
}

// CleanupAppCertificates removes every trace this application left behind:
//   - the debug root CA(s) from the OS trust store and every per-domain leaf they
//     issued (matched by caCommonNameMarker), when the OS is supported;
//   - the per-domain certificates folder;
//   - the root CA certificate and key files.
//
// It is best-effort: a failure on one item is recorded in the report's Warnings
// rather than aborting the rest. The returned error is non-nil only for an
// unexpected, non-recoverable condition (currently none — callers can rely on
// the report).
func CleanupAppCertificates(certConfig configuration.CertManagerConfig, installer CertInstaller) (CleanupReport, error) {
	report := CleanupReport{}

	// 1. OS trust store.
	if installer != nil {
		root, domain, supported, err := installer.CleanupStore(caCommonNameMarker)
		report.RootCertsRemoved = root
		report.DomainCertsRemoved = domain
		report.StoreCleanupSupported = supported
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("trust store cleanup: %v", err))
		}
	}

	// 2. Per-domain certificates folder.
	if folder := strings.TrimSpace(certConfig.DomainCertsFolder); folder != "" {
		removed, err := removeDomainCertsFolder(folder)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("domain certs folder %q: %v", folder, err))
		}
		report.DomainFolderRemoved = removed
	}

	// 3. Root CA certificate + key files.
	for _, file := range []string{certConfig.CaCertFile, certConfig.CaKeyFile} {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		if err := os.Remove(file); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				report.Warnings = append(report.Warnings, fmt.Sprintf("%s: %v", file, err))
			}
			continue
		}
		report.RemovedFiles = append(report.RemovedFiles, file)
	}

	return report, nil
}

// removeDomainCertsFolder deletes the per-domain certificates folder, returning
// whether it removed anything. A missing folder is not an error. The folder path
// is guarded against obviously dangerous values (empty, ".", a filesystem or
// volume root) so a misconfiguration can never wipe an unintended tree.
func removeDomainCertsFolder(folder string) (bool, error) {
	if !safeToRemove(folder) {
		return false, fmt.Errorf("refusing to remove unsafe path")
	}
	if _, err := os.Stat(folder); errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err := os.RemoveAll(folder); err != nil {
		return false, err
	}
	return true, nil
}

// safeToRemove rejects paths that must never be handed to os.RemoveAll: empty,
// the current directory, a filesystem root, or a volume root such as "C:\".
func safeToRemove(path string) bool {
	clean := filepath.Clean(path)
	if clean == "" || clean == "." {
		return false
	}
	if clean == filepath.Dir(clean) {
		// Filesystem root ("/", or "C:\" whose parent is itself).
		return false
	}
	if vol := filepath.VolumeName(clean); vol != "" && clean == vol+string(filepath.Separator) {
		return false
	}
	return true
}
