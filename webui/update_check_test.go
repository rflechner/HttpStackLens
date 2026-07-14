package webui

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.2.0", "v0.1.0", true},
		{"v0.1.0", "v0.1.0", false},
		{"v0.1.0", "v0.2.0", false},
		{"v0.10.0", "v0.9.0", true},        // not lexical
		{"v1.0.0", "v0.20.30", true},       // major wins
		{"v0.2.1", "v0.2.0", true},         // patch
		{"v0.2.0", "v0.2.0-3-gabc", true},  // release beats between-tags build
		{"v0.2.0-3-gabc", "v0.2.0", false}, // pre-release never newer than release
		{"v0.2.0", "dev", false},           // dev build: never "newer"
		{"not-a-version", "v0.1.0", false},
	}
	for _, c := range cases {
		if got := isNewer(c.latest, c.current); got != c.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestDisabledCheckerDoesNotContactGitHub(t *testing.T) {
	// A nil HTTP client would panic on any outbound call, proving the disabled
	// checker short-circuits before touching the network.
	c := newUpdateChecker(false, "v0.1.0", "rflechner/HttpStackLens")
	c.client = nil
	got := c.result()
	if got.Checked || got.UpdateAvailable {
		t.Errorf("disabled checker returned %+v, want Checked=false", got)
	}
}

func TestParseSemver(t *testing.T) {
	if _, _, ok := parseSemver("dev"); ok {
		t.Error("parseSemver(dev) should be invalid")
	}
	if _, pre, ok := parseSemver("v1.2.3+build"); !ok || pre {
		t.Error("v1.2.3+build should be valid, non-prerelease")
	}
	if core, pre, ok := parseSemver("v1.2.3-rc1"); !ok || !pre || core != [3]int{1, 2, 3} {
		t.Errorf("v1.2.3-rc1 parse = %v pre=%v ok=%v", core, pre, ok)
	}
}
