package certManager

// CertInstaller installs the debug certificates into the operating system trust
// stores, so that intercepted HTTPS traffic is trusted without any extra manual
// step.
//
// Implementations are platform-specific and selected at build time. On an
// operating system that has no implementation, NewCertInstaller returns a no-op
// installer (IsSupported reports false) and the user is expected to install the
// certificates by hand.
type CertInstaller interface {
	// InstallCACert adds the CA certificate to the trust store that holds the
	// other trusted certificate authorities.
	InstallCACert(caCertFile string) error

	// InstallDomainCert adds a signed per-domain certificate to the current
	// user's personal certificate store.
	InstallDomainCert(domainCertFile string) error

	// IsSupported reports whether certificate installation is implemented for
	// the current operating system.
	IsSupported() bool
}

// noopInstaller is used on operating systems without a native implementation:
// every operation succeeds while doing nothing.
type noopInstaller struct{}

func (noopInstaller) InstallCACert(string) error     { return nil }
func (noopInstaller) InstallDomainCert(string) error { return nil }
func (noopInstaller) IsSupported() bool              { return false }
