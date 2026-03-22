//go:build windows

package security

import (
	"encoding/base64"
	"fmt"
	"log"

	"github.com/alexbrainman/sspi"
	"github.com/alexbrainman/sspi/negotiate"
)

func getNtlmHeaderToken(spn string) (string, error) {
	// get Windows user credentials from  SSPI
	credentials, err := negotiate.AcquireCurrentUserCredentials()
	if err != nil {
		log.Fatalf("Could not get current user Windows credentials : %v", err)
	}
	defer func(credentials *sspi.Credentials) {
		err := credentials.Release()
		if err != nil {
			fmt.Printf("Could not release Windows credentials : %v", err)
		}
	}(credentials)

	// Init client NTLM context
	clientContext, negotiateToken, err := negotiate.NewClientContext(credentials, spn)
	if err != nil {
		log.Fatalf("Could not initialize NTLM : %v", err)
	}
	defer func(clientContext *negotiate.ClientContext) {
		err := clientContext.Release()
		if err != nil {
			fmt.Printf("Could not release NTLM context : %v", err)
		}
	}(clientContext)

	// TODO: challenge with server

	// 3. Generate a first NTLM token (Type 1: Negotiate)
	//negotiateToken, err := clientContext.Update(nil)
	//if err != nil {
	//	log.Fatalf("Could not generate NTLM token : %v", err)
	//}

	tokenBase64 := base64.StdEncoding.EncodeToString(negotiateToken)

	return tokenBase64, nil
}
