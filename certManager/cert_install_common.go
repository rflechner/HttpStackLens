package certManager

import (
	"encoding/pem"
	"os"
)

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
