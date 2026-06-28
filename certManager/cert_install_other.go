//go:build !windows && !darwin

package certManager

// NewCertInstaller returns a no-op installer on operating systems that have no
// native trust-store integration yet. Certificates must be installed manually
// by the user.
func NewCertInstaller() CertInstaller {
	return noopInstaller{}
}
