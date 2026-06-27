package certManager

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"httpStackLens/configuration"
	"log"
	"math/big"
	"os"
	"time"
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func GetHttpsDebugCertificates(config configuration.AppConfig) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	if config.CertManager.CaCertFile == "" || config.CertManager.CaKeyFile == "" {
		log.Fatal("CA certificate and key files must be specified in config.yaml")
		return nil, nil, errors.New("CA certificate and key files must be specified in config.yaml")
	}

	if !fileExists(config.CertManager.CaCertFile) || !fileExists(config.CertManager.CaKeyFile) {
		err := GenerateCA(config.CertManager.CaCertFile, config.CertManager.CaKeyFile)
		if err != nil {
			log.Printf("Failed to generate CA: %v\n", err)
			return nil, nil, err
		}
	} else {
		log.Printf("🔒 CA certificate and key files already exist, skipping generation")
	}

	caCert, caKey, err := LoadCA(config.CertManager.CaCertFile, config.CertManager.CaKeyFile)
	if err != nil {
		log.Printf("Failed to load CA: %v\n", err)
		return nil, nil, err
	}
	//_, _, err = certManager.SignServerCert(caCert, caKey, []string{"example.com", "www.example.com"})
	//if err != nil {
	//	log.Printf("Failed to sign server certificate: %v\n", err)
	//	return
	//}

	return caCert, caKey, nil
}

func GenerateCA(certFile string, keyFile string) error {
	// 1. Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	log.Println("🔏 Generating CA for debugging HTTPS on machine 🔛 " + hostname)

	// 2. Describe CA
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   hostname + " - My Local CA for debugging HTTPS",
			Organization: []string{hostname + "Debug HTTPS"},
			Country:      []string{"Unknown"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour), // 10 ans
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	// 3. Auto-sign (parent == template, clé == private key)
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return err
	}

	// 4. Write PEM file
	certOut, err := os.Create(certFile)
	if err != nil {
		return err
	}
	defer certOut.Close()
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err != nil {
		return err
	}

	// 5. Write the private key in PEM
	keyOut, err := os.Create(keyFile)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return err
	}
	err = pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err != nil {
		return err
	}

	certFileFullPath, _ := os.Getwd()
	certFileFullPath = certFileFullPath + string(os.PathSeparator) + certFile
	keyFileFullPath, _ := os.Getwd()
	keyFileFullPath = keyFileFullPath + string(os.PathSeparator) + keyFile

	log.Println("🔒 CA generated successfully for debugging HTTPS on machine 🔛 " + hostname)
	log.Println("🔒 CA certificate and private key files created at " + certFileFullPath + " and " + keyFileFullPath)

	return nil
}

func LoadCA(certFile, keyFile string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	// Read certificate
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, nil, err
	}
	block, _ := pem.Decode(certPEM)
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, err
	}

	// Read private key
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, nil, err
	}
	block, _ = pem.Decode(keyPEM)
	caKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, err
	}

	return caCert, caKey, nil
}

func SignServerCert(ca *x509.Certificate, caKey *ecdsa.PrivateKey, domains []string) ([]byte, *ecdsa.PrivateKey, error) {
	serverKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: domains[0]},
		DNSNames:     domains,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	// Here parent = CA -> signed by CA
	certDER, err := x509.CreateCertificate(rand.Reader, template, ca, &serverKey.PublicKey, caKey)
	return certDER, serverKey, err
}
