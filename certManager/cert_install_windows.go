//go:build windows

package certManager

import (
	"fmt"
	"log"
	"syscall"
	"unsafe"
)

// Windows certificate-store API, exposed by crypt32.dll. We call it directly
// instead of shelling out to certutil, so the install is a plain in-process
// API call with no external dependency.
var (
	crypt32                              = syscall.NewLazyDLL("crypt32.dll")
	procCertOpenStore                    = crypt32.NewProc("CertOpenStore")
	procCertCloseStore                   = crypt32.NewProc("CertCloseStore")
	procCertAddEncodedCertificateToStore = crypt32.NewProc("CertAddEncodedCertificateToStore")
	procCertCreateCertificateContext     = crypt32.NewProc("CertCreateCertificateContext")
	procCertFindCertificateInStore       = crypt32.NewProc("CertFindCertificateInStore")
	procCertFreeCertificateContext       = crypt32.NewProc("CertFreeCertificateContext")
)

const (
	// CERT_STORE_PROV_SYSTEM_W: the store name is a wide string.
	certStoreProvSystemW = 10
	// CERT_SYSTEM_STORE_CURRENT_USER: per-user stores, no admin rights needed.
	certSystemStoreCurrentUser = 0x00010000
	// Encoding of the certificate bytes we hand to the API.
	x509AsnEncoding  = 0x00000001
	pkcs7AsnEncoding = 0x00010000
	// CERT_STORE_ADD_REPLACE_EXISTING: overwrite a previous entry instead of failing.
	certStoreAddReplaceExisting = 3
	// CERT_FIND_EXISTING ((CERT_COMPARE_EXISTING=13) << CERT_COMPARE_SHIFT=16):
	// find a cert context in the store identical to the one supplied.
	certFindExisting = 13 << 16
)

const certEncoding = x509AsnEncoding | pkcs7AsnEncoding

// NewCertInstaller returns the Windows certificate installer.
func NewCertInstaller() CertInstaller {
	return windowsCertInstaller{}
}

type windowsCertInstaller struct{}

func (windowsCertInstaller) IsSupported() bool { return true }

func (windowsCertInstaller) IsCACertInstalled(caCertFile string) (bool, error) {
	der, err := readCertDER(caCertFile)
	if err != nil {
		return false, err
	}
	if len(der) == 0 {
		return false, fmt.Errorf("no certificate found in %q", caCertFile)
	}
	return certInStore("ROOT", der)
}

// InstallCACert imports the CA certificate into the current user's "Root" store,
// next to the other trusted certificate authorities. It first checks whether the
// exact certificate is already present and skips the add if so — adding to the
// Root store otherwise pops a Windows security prompt on every launch.
func (windowsCertInstaller) InstallCACert(caCertFile string) error {
	der, err := readCertDER(caCertFile)
	if err != nil {
		return err
	}
	if len(der) == 0 {
		return fmt.Errorf("no certificate found in %q", caCertFile)
	}

	exists, err := certInStore("ROOT", der)
	if err != nil {
		// Don't fail the launch on a check error; fall through and try to add.
		log.Printf("⚠️  Could not check the Root store for the CA certificate: %v\n", err)
	} else if exists {
		log.Printf("🔒 CA certificate already trusted in the current user's Root store, skipping: %s\n", caCertFile)
		return nil
	}

	if err := addEncodedCertToStore("ROOT", der); err != nil {
		return err
	}
	log.Printf("🔒 CA certificate installed in the current user's Root store: %s\n", caCertFile)
	return nil
}

// InstallDomainCert imports a signed domain certificate into the current user's
// personal ("My") store.
func (windowsCertInstaller) InstallDomainCert(domainCertFile string) error {
	der, err := readCertDER(domainCertFile)
	if err != nil {
		return err
	}
	if len(der) == 0 {
		return fmt.Errorf("no certificate found in %q", domainCertFile)
	}
	if err := addEncodedCertToStore("MY", der); err != nil {
		return err
	}
	log.Printf("🔏 Domain certificate installed in the current user's personal store: %s\n", domainCertFile)
	return nil
}

// openCurrentUserStore opens a named current-user system store (e.g. "ROOT" or
// "MY"). The caller must close the returned handle with CertCloseStore.
func openCurrentUserStore(storeName string) (uintptr, error) {
	storeNamePtr, err := syscall.UTF16PtrFromString(storeName)
	if err != nil {
		return 0, err
	}
	store, _, callErr := procCertOpenStore.Call(
		uintptr(certStoreProvSystemW),
		0,
		0,
		uintptr(certSystemStoreCurrentUser),
		uintptr(unsafe.Pointer(storeNamePtr)),
	)
	if store == 0 {
		return 0, fmt.Errorf("CertOpenStore(%s) failed: %v", storeName, callErr)
	}
	return store, nil
}

// addEncodedCertToStore adds a DER-encoded certificate to the named store.
func addEncodedCertToStore(storeName string, der []byte) error {
	store, err := openCurrentUserStore(storeName)
	if err != nil {
		return err
	}
	defer procCertCloseStore.Call(store, 0)

	ret, _, callErr := procCertAddEncodedCertificateToStore.Call(
		store,
		uintptr(certEncoding),
		uintptr(unsafe.Pointer(&der[0])),
		uintptr(len(der)),
		uintptr(certStoreAddReplaceExisting),
		0,
	)
	if ret == 0 {
		return fmt.Errorf("CertAddEncodedCertificateToStore(%s) failed: %v", storeName, callErr)
	}
	return nil
}

// certInStore reports whether a certificate identical to der is already present
// in the named current-user store.
func certInStore(storeName string, der []byte) (bool, error) {
	store, err := openCurrentUserStore(storeName)
	if err != nil {
		return false, err
	}
	defer procCertCloseStore.Call(store, 0)

	// Wrap the DER bytes in a cert context to compare against the store.
	ctx, _, callErr := procCertCreateCertificateContext.Call(
		uintptr(certEncoding),
		uintptr(unsafe.Pointer(&der[0])),
		uintptr(len(der)),
	)
	if ctx == 0 {
		return false, fmt.Errorf("CertCreateCertificateContext failed: %v", callErr)
	}
	defer procCertFreeCertificateContext.Call(ctx)

	found, _, _ := procCertFindCertificateInStore.Call(
		store,
		uintptr(certEncoding),
		0,
		uintptr(certFindExisting),
		ctx,
		0,
	)
	if found != 0 {
		// CertFindCertificateInStore returns a context the caller must free.
		procCertFreeCertificateContext.Call(found)
		return true, nil
	}
	return false, nil
}
