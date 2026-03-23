//go:build windows

package security

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/alexbrainman/sspi"
	"github.com/alexbrainman/sspi/kerberos"
	"github.com/alexbrainman/sspi/negotiate"
	"github.com/alexbrainman/sspi/ntlm"
)

type AuthPackage int

const (
	AuthNone AuthPackage = iota
	AuthKerberos
	AuthNTLM
	AuthNegotiate
)

func (a AuthPackage) String() string {
	switch a {
	case AuthNone:
		return "None"
	case AuthNTLM:
		return "NTLM"
	case AuthNegotiate:
		return "Negotiate"
	case AuthKerberos:
		return "Kerberos"
	default:
		return ""
	}
}

func ParseAuthPackage(text string) (AuthPackage, error) {
	if strings.ToUpper(text) == "NTLM" {
		return AuthNTLM, nil
	}
	if strings.ToLower(text) == "negotiate" {
		return AuthNegotiate, nil
	}
	if strings.ToLower(text) == "kerberos" {
		return AuthKerberos, nil
	}
	return AuthNone, fmt.Errorf("unknown auth package: %s", text)
}

type ServerAuth struct {
	Credentials *sspi.Credentials
	AuthPackage AuthPackage
}

func NewServerAuth(authPackage AuthPackage) (*ServerAuth, error) {
	if authPackage == AuthNTLM {
		return newNtlmServerAuth()
	}
	if authPackage == AuthNegotiate {
		return newNegotiateServerAuth()
	}
	if authPackage == AuthKerberos {
		return newKerberosServerAuth()
	}

	return nil, fmt.Errorf("unsupported auth package: %s", authPackage)
}

func getServerSPN() string {
	hostname, _ := os.Hostname()
	return "HTTP/" + hostname
}

func newNegotiateServerAuth() (*ServerAuth, error) {
	spn := getServerSPN()
	cred, err := negotiate.AcquireServerCredentials(spn)
	if err != nil {
		return nil, err
	}
	return &ServerAuth{
		Credentials: cred,
		AuthPackage: AuthNegotiate,
	}, nil
}

func newNtlmServerAuth() (*ServerAuth, error) {
	cred, err := ntlm.AcquireServerCredentials()
	if err != nil {
		return nil, err
	}
	return &ServerAuth{
		Credentials: cred,
		AuthPackage: AuthNTLM,
	}, nil
}

func newKerberosServerAuth() (*ServerAuth, error) {
	spn := getServerSPN()
	cred, err := kerberos.AcquireServerCredentials(spn)
	if err != nil {
		return nil, err
	}
	return &ServerAuth{
		Credentials: cred,
		AuthPackage: AuthKerberos,
	}, nil
}

func (a *ServerAuth) Release() error {
	err := a.Credentials.Release()
	if err != nil {
		return err
	}
	return nil
}

func (a *ServerAuth) ValidateToken(token []byte) (authDone bool, outputToken []byte, err error) {

	debugToken(token)

	switch a.AuthPackage {
	case AuthNone:
		return false, nil, fmt.Errorf("unsupported auth package: %s", AuthNone)
	case AuthNegotiate:
		ctx, authDone, outputToken, err := negotiate.NewServerContext(a.Credentials, token)
		if err != nil {
			return false, nil, err
		}
		defer ctx.Release()

		return authDone, outputToken, nil
	case AuthKerberos:
		ctx, authDone, outputToken, err := kerberos.NewServerContext(a.Credentials, token)
		if err != nil {
			return false, nil, err
		}
		defer ctx.Release()

		return authDone, outputToken, nil
	case AuthNTLM:
		ctx, outputToken, err := ntlm.NewServerContext(a.Credentials, token)
		if err != nil {
			return false, nil, err
		}
		defer ctx.Release()

		return true, outputToken, nil
	}

	return false, nil, fmt.Errorf("unsupported auth package: %s", a.AuthPackage)
}

func debugToken(token []byte) {
	// Les 8 premiers octets identifient le mécanisme
	// NTLMSSP\x00 → NTLM
	// \x60 → SPNEGO/Kerberos (ASN.1)
	if len(token) > 7 && string(token[:7]) == "NTLMSSP" {
		log.Println("→ token NTLM")
	} else if len(token) > 0 && token[0] == 0x60 {
		log.Println("→ token SPNEGO/Kerberos")
	}
}
