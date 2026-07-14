package storage

import "sync"

// CaptureController is the shared runtime switch for live capture. Turning it
// off must not affect proxy forwarding; it only gates UI events and recording.
type CaptureController struct {
	mu        sync.RWMutex
	capturing bool
}

func NewCaptureController(capturing bool) *CaptureController {
	return &CaptureController{capturing: capturing}
}

func (c *CaptureController) IsCapturing() bool {
	if c == nil {
		return true
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.capturing
}

func (c *CaptureController) Pause() bool {
	return c.set(false)
}

func (c *CaptureController) Resume() bool {
	return c.set(true)
}

func (c *CaptureController) set(capturing bool) bool {
	if c == nil {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.capturing = capturing
	return c.capturing
}
