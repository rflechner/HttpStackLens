package storage

import "testing"

func TestCaptureControllerPauseResume(t *testing.T) {
	c := NewCaptureController(true)
	if !c.IsCapturing() {
		t.Fatal("new controller should start capturing")
	}

	if got := c.Pause(); got {
		t.Fatal("Pause returned capturing=true")
	}
	if c.IsCapturing() {
		t.Fatal("controller still capturing after Pause")
	}

	if got := c.Resume(); !got {
		t.Fatal("Resume returned capturing=false")
	}
	if !c.IsCapturing() {
		t.Fatal("controller not capturing after Resume")
	}
}
