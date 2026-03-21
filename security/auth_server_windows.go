//go:build windows

package security

import (
	"fmt"
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

func newNegotiateServerAuth() (*ServerAuth, error) {
	cred, err := negotiate.AcquireServerCredentials("")
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
	cred, err := kerberos.AcquireServerCredentials("")
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
