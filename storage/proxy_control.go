package storage

import "sync"

// ProxyController exposes the runtime listener state to the Web UI. The
// runtime supervisor owns the actual listener lifecycle and updates this value
// only after a start or stop operation succeeds.
type ProxyController struct {
	mu      sync.RWMutex
	running bool
}

func NewProxyController(running bool) *ProxyController {
	return &ProxyController{running: running}
}

func (c *ProxyController) IsRunning() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}

func (c *ProxyController) SetRunning(running bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.running = running
	c.mu.Unlock()
}
