package configuration

import (
	"testing"

	"github.com/goccy/go-yaml"
)

func TestCaptureMimeTypeLimits(t *testing.T) {
	const data = `
decrypt_https: true
mime_types:
  - name: "image/*"
    max_size_mb: 2.5
  - name: "text/*"
    max_size_kb: 10000
  - name: "application/json"
`
	var c CaptureConfig
	if err := yaml.Unmarshal([]byte(data), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !c.DecryptHttps {
		t.Fatalf("decrypt_https = false, want true")
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
