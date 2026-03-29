//go:build windows

package security

import (
	"fmt"

	"github.com/alexbrainman/sspi"
	"github.com/alexbrainman/sspi/kerberos"
	"github.com/alexbrainman/sspi/negotiate"
	"github.com/alexbrainman/sspi/ntlm"
)

type ClientAuth struct {
	Credentials *sspi.Credentials
	AuthPackage AuthPackage
	Context     any
}

func NewClientAuth(authPackage AuthPackage) (*ClientAuth, error) {
	var cred *sspi.Credentials
	var err error

	switch authPackage {
	case AuthNTLM:
		cred, err = ntlm.AcquireCurrentUserCredentials()
	case AuthNegotiate:
		cred, err = negotiate.AcquireCurrentUserCredentials()
	case AuthKerberos:
		cred, err = kerberos.AcquireCurrentUserCredentials()
	default:
		return nil, fmt.Errorf("unsupported auth package: %v", authPackage)
	}

	if err != nil {
		return nil, err
	}

	return &ClientAuth{
		Credentials: cred,
		AuthPackage: authPackage,
	}, nil
}

func (a *ClientAuth) Release() error {
	if a.Credentials != nil {
		if err := a.Credentials.Release(); err != nil {
			return err
		}
	}
	// Note: ClientContext release is handled internally or needs to be tracked if we keep it
	return nil
}

func (a *ClientAuth) Update(token []byte) (authDone bool, outputToken []byte, err error) {
	switch a.AuthPackage {
	case AuthNTLM:
		var ctx *ntlm.ClientContext
		if a.Context == nil {
			ctx, outputToken, err = ntlm.NewClientContext(a.Credentials)
			if err != nil {
				return false, nil, err
			}
			a.Context = ctx
		} else {
			ctx = a.Context.(*ntlm.ClientContext)
			outputToken, err = ctx.Update(token)
			if err != nil {
				return false, nil, err
			}
		}
		// NTLM is usually done after the second update (Type 3 message)
		// But sspi package might handle it differently.
		// For simplicity, we assume if err is nil, it's either done or needs more.
		// Usually: Update(nil) -> Type 1, Update(Type 2) -> Type 3.
		return token != nil, outputToken, nil

	case AuthNegotiate:
		var ctx *negotiate.ClientContext
		if a.Context == nil {
			var negotiateToken []byte
			ctx, negotiateToken, err = negotiate.NewClientContext(a.Credentials, "")
			if err != nil {
				return false, nil, err
			}
			a.Context = ctx
			return false, negotiateToken, nil
		} else {
			ctx = a.Context.(*negotiate.ClientContext)
			authDone, outputToken, err = ctx.Update(token)
			return authDone, outputToken, err
		}
	case AuthKerberos:
		var ctx *kerberos.ClientContext
		if a.Context == nil {
			var kerberosToken []byte
			var authDone bool
			ctx, authDone, kerberosToken, err = kerberos.NewClientContext(a.Credentials, "")
			if err != nil {
				return false, nil, err
			}
			a.Context = ctx
			return authDone, kerberosToken, nil
		} else {
			ctx = a.Context.(*kerberos.ClientContext)
			authDone, outputToken, err = ctx.Update(token)
			return authDone, outputToken, err
		}
	}

	return false, nil, fmt.Errorf("unsupported auth package")
}
