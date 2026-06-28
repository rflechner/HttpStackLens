//go:build darwin

package certManager

func NewCertInstaller() CertInstaller {
	return macOsCertInstaller{}
}

// macOsCertInstaller is used on Mac OS
type macOsCertInstaller struct{}

func (macOsCertInstaller) InstallCACert(string) error {
	return nil
}

func (macOsCertInstaller) InstallDomainCert(string) error {
	return nil
}

func (macOsCertInstaller) IsSupported() bool {
	return true
}
