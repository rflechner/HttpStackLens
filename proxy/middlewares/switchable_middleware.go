package middlewares

import (
	"httpStackLens/http/models"
	"net"
	"sync/atomic"
)

// SwitchableMiddleware forwards requests to the currently active middleware.
// Updating the target only affects new proxy requests; connections already
// flowing through the previous middleware keep using it until they finish.
type SwitchableMiddleware struct {
	target atomic.Value
}

type switchableTarget struct {
	middleware   Middleware
	decryptHttps bool
}

func NewSwitchableMiddleware(initial Middleware) *SwitchableMiddleware {
	m := &SwitchableMiddleware{}
	m.Set(initial)
	return m
}

func (m *SwitchableMiddleware) Set(next Middleware) {
	m.SetDecrypting(next, false)
}

func (m *SwitchableMiddleware) SetDecrypting(next Middleware, decryptHttps bool) {
	m.target.Store(switchableTarget{middleware: next, decryptHttps: decryptHttps})
}

func (m *SwitchableMiddleware) Get() Middleware {
	next, _ := m.Snapshot()
	return next
}

func (m *SwitchableMiddleware) Snapshot() (Middleware, bool) {
	if target, ok := m.target.Load().(switchableTarget); ok {
		return target.middleware, target.decryptHttps
	}
	return nil, false
}

func (m *SwitchableMiddleware) HandleProxyRequest(browser net.Conn, request models.ProxyRequest) error {
	return m.Get().HandleProxyRequest(browser, request)
}
