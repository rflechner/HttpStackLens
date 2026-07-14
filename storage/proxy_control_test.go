package storage

import "testing"

func TestProxyControllerTracksRunningState(t *testing.T) {
	controller := NewProxyController(true)
	if !controller.IsRunning() {
		t.Fatal("proxy should start running")
	}
	controller.SetRunning(false)
	if controller.IsRunning() {
		t.Fatal("proxy should be stopped")
	}
	controller.SetRunning(true)
	if !controller.IsRunning() {
		t.Fatal("proxy should be running again")
	}
}
