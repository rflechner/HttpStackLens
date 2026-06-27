//go:build windows

package certManager

import (
	"encoding/pem"
	"fmt"
	"log"
	"os"
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
)

// NewCertInstaller returns the Windows certificate installer.
func NewCertInstaller() CertInstaller {
	return windowsCertInstaller{}
}

type windowsCertInstaller struct{}

func (windowsCertInstaller) IsSupported() bool { return true }

// InstallCACert imports the CA certificate into the current user's "Root" store,
// next to the other trusted certificate authorities.
func (windowsCertInstaller) InstallCACert(caCertFile string) error {
	if err := addCertToStore("ROOT", caCertFile); err != nil {
		return err
	}
	log.Printf("🔒 CA certificate installed in the current user's Root store: %s\n", caCertFile)
	return nil
}

// InstallDomainCert imports a signed domain certificate into the current user's
// personal ("My") store.
func (windowsCertInstaller) InstallDomainCert(domainCertFile string) error {
	if err := addCertToStore("MY", domainCertFile); err != nil {
		return err
	}
	log.Printf("🔏 Domain certificate installed in the current user's personal store: %s\n", domainCertFile)
	return nil
}

// addCertToStore adds the certificate found in certFile to the named current-user
// system store (e.g. "ROOT" or "MY") using the crypt32 API.
func addCertToStore(storeName, certFile string) error {
	der, err := readCertDER(certFile)
	if err != nil {
		return err
	}
	if len(der) == 0 {
		return fmt.Errorf("no certificate found in %q", certFile)
	}

	storeNamePtr, err := syscall.UTF16PtrFromString(storeName)
	if err != nil {
		return err
	}

	store, _, callErr := procCertOpenStore.Call(
		uintptr(certStoreProvSystemW),
		0,
		0,
		uintptr(certSystemStoreCurrentUser),
		uintptr(unsafe.Pointer(storeNamePtr)),
	)
	if store == 0 {
		return fmt.Errorf("CertOpenStore(%s) failed: %v", storeName, callErr)
	}
	defer procCertCloseStore.Call(store, 0)

	ret, _, callErr := procCertAddEncodedCertificateToStore.Call(
		store,
		uintptr(x509AsnEncoding|pkcs7AsnEncoding),
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

// readCertDER returns the DER bytes of the certificate in certFile, accepting
// either a PEM-encoded file (our case) or an already DER-encoded one.
func readCertDER(certFile string) ([]byte, error) {
	data, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}
	if block, _ := pem.Decode(data); block != nil {
		return block.Bytes, nil
	}
	return data, nil
}
