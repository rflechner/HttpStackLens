package middlewares

import (
	"httpStackLens/http/models"
	"net"
	"testing"
)

type noopMiddleware struct{}

func (noopMiddleware) HandleProxyRequest(net.Conn, models.ProxyRequest) error {
	return nil
}

func TestSwitchableMiddlewareSnapshotTracksActiveTargetAndMode(t *testing.T) {
	plain := noopMiddleware{}
	decrypted := noopMiddleware{}
	switchable := NewSwitchableMiddleware(plain)

	got, decrypting := switchable.Snapshot()
	if got == nil || decrypting {
		t.Fatalf("initial snapshot = (%T, %v), want plain/false", got, decrypting)
	}

	switchable.SetDecrypting(decrypted, true)
	got, decrypting = switchable.Snapshot()
	if got == nil || !decrypting {
		t.Fatalf("updated snapshot = (%T, %v), want decrypted/true", got, decrypting)
	}
}
