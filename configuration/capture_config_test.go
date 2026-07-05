package configuration

import (
	"testing"

	"github.com/goccy/go-yaml"
)

func TestCaptureMimeTypeLimits(t *testing.T) {
	const data = `
enabled: true
mime_types:
  - name: "image/*"
    max_size_mb: 2.5
  - name: "text/*"
    max_size_kb: 10000
  - name: "application/json"
`
	var c DecryptHttpsConfig
	if err := yaml.Unmarshal([]byte(data), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !c.Enabled {
		t.Fatalf("enabled = false, want true")
	}

	cases := []struct {
		contentType string
		wantLimit   int64
		wantMatched bool
	}{
		{"image/png", int64(2.5 * 1024 * 1024), true},                // image/* via mb
		{"text/html; charset=utf-8", 10000 * 1024, true},             // text/* via kb, params stripped
		{"application/json", DefaultCaptureSizeBytes, true},          // explicit rule, no size -> default
		{"application/octet-stream", DefaultCaptureSizeBytes, false}, // no rule -> default, unmatched
	}
	for _, tc := range cases {
		limit, matched := c.LimitForContentType(tc.contentType)
		if limit != tc.wantLimit || matched != tc.wantMatched {
			t.Errorf("LimitForContentType(%q) = (%d, %v), want (%d, %v)",
				tc.contentType, limit, matched, tc.wantLimit, tc.wantMatched)
		}
	}
}

func TestCaptureDefaultMaxBytes(t *testing.T) {
	const data = `
default_max_bytes: 1048576
mime_types:
  - name: "image/*"
    max_size_mb: 2
  - name: "text/*"
`
	var c DecryptHttpsConfig
	if err := yaml.Unmarshal([]byte(data), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Explicit rule size wins.
	if limit, _ := c.LimitForContentType("image/jpeg"); limit != 2*1024*1024 {
		t.Errorf("image/jpeg = %d, want %d", limit, 2*1024*1024)
	}
	// Matched rule without a size falls back to default_max_bytes.
	if limit, matched := c.LimitForContentType("text/css"); limit != 1048576 || !matched {
		t.Errorf("text/css = (%d, %v), want (1048576, true)", limit, matched)
	}
	// Unmatched type also uses default_max_bytes.
	if limit, matched := c.LimitForContentType("video/mp4"); limit != 1048576 || matched {
		t.Errorf("video/mp4 = (%d, %v), want (1048576, false)", limit, matched)
	}
}

func TestMimeTypeRuleLimitBytesUnits(t *testing.T) {
	bytes := int64(1234)
	kb := 8.0
	mb := 1.5

	cases := []struct {
		name string
		rule MimeTypeRule
		want int64
	}{
		{"bytes", MimeTypeRule{MaxSizeBytes: &bytes}, 1234},
		{"kb", MimeTypeRule{MaxSizeKb: &kb}, 8 * 1024},
		{"mb", MimeTypeRule{MaxSizeMb: &mb}, int64(1.5 * 1024 * 1024)},
		{"default", MimeTypeRule{}, DefaultCaptureSizeBytes},
	}
	for _, tc := range cases {
		if got := tc.rule.LimitBytes(); got != tc.want {
			t.Errorf("%s: LimitBytes() = %d, want %d", tc.name, got, tc.want)
		}
	}
}
