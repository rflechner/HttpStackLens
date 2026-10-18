package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"httpStackLens/storage"
	"net/url"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type SearchParams struct {
	Host         string `json:"host,omitempty" jsonschema:"Exact hostname, case insensitive, without port; does not include subdomains automatically"`
	Method       string `json:"method,omitempty" jsonschema:"HTTP method, case insensitive"`
	PathContains string `json:"path_contains,omitempty" jsonschema:"Case sensitive substring of the URL path"`
	StatusCode   int    `json:"status_code,omitempty" jsonschema:"Exact HTTP status from 100 to 599; omitted or zero means any"`
	HasResponse  *bool  `json:"has_response,omitempty" jsonschema:"Filter by presence of a recorded response; false does not prove a network error"`
	Since        string `json:"since,omitempty" jsonschema:"Inclusive RFC3339 lower bound on first observation time"`
	Until        string `json:"until,omitempty" jsonschema:"Inclusive RFC3339 upper bound on first observation time"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Page size, default 25, maximum 100"`
	Cursor       string `json:"cursor,omitempty" jsonschema:"Opaque next_cursor from previous page; repeat the same filters"`
}

type RequestSummary struct {
	ID           string    `json:"id"`
	ObservedAt   time.Time `json:"observed_at"`
	Method       string    `json:"method"`
	URL          string    `json:"url" jsonschema:"Userinfo and fragment removed, all query values redacted; paths may still contain sensitive data"`
	URLRedacted  bool      `json:"url_redacted"`
	URLTruncated bool      `json:"url_truncated"`
	StatusCode   *int16    `json:"status_code"`
	HasRequest   bool      `json:"has_request"`
	HasResponse  bool      `json:"has_response"`
	DurationMS   *float64  `json:"duration_ms" jsonschema:"Measured total duration when available; null otherwise"`
	Visibility   string    `json:"visibility" jsonschema:"connect (inner traffic not shown here), https_request, http_request, or unknown"`
}

type SearchResult struct {
	Source      string           `json:"source"`
	Requests    []RequestSummary `json:"requests"`
	NextCursor  string           `json:"next_cursor,omitempty"`
	Limitations []string         `json:"limitations"`
}

type searchCursor struct {
	Epoch      string
	Generation uint64
	Before     uint64
	Filters    [32]byte
}

func (d Dependencies) searchRequests(ctx context.Context, _ *sdk.CallToolRequest, p SearchParams) (*sdk.CallToolResult, SearchResult, error) {
	fail := func(msg string) (*sdk.CallToolResult, SearchResult, error) {
		return nil, SearchResult{}, fmt.Errorf("%s", msg)
	}
	if d.Requests == nil {
		return fail("request store unavailable")
	}
	if p.Limit < 0 || p.Limit > 100 {
		return fail("limit must be between 1 and 100, or 0 for the default")
	}
	if p.Limit == 0 {
		p.Limit = 25
	}
	if p.StatusCode != 0 && (p.StatusCode < 100 || p.StatusCode > 599) {
		return fail("status_code must be between 100 and 599")
	}
	if len(p.Host) > 253 || len(p.Method) > 32 || len(p.PathContains) > 2048 || len(p.Cursor) > 2048 {
		return fail("search parameter too long")
	}
	var since, until time.Time
	var err error
	if p.Since != "" {
		since, err = time.Parse(time.RFC3339Nano, p.Since)
		if err != nil {
			return fail("since must be RFC3339")
		}
	}
	if p.Until != "" {
		until, err = time.Parse(time.RFC3339Nano, p.Until)
		if err != nil {
			return fail("until must be RFC3339")
		}
	}
	if !since.IsZero() && !until.IsZero() && since.After(until) {
		return fail("since must not be after until")
	}
	p.Host = strings.ToLower(strings.TrimSpace(p.Host))
	p.Method = strings.ToUpper(strings.TrimSpace(p.Method))
	filters := p
	filters.Cursor = ""
	filters.Limit = 0
	encoded, _ := json.Marshal(filters)
	fingerprint := sha256.Sum256(encoded)
	entries, epoch, generation := d.Requests.Snapshot()
	cursor := searchCursor{Epoch: epoch, Generation: generation, Filters: fingerprint}
	if p.Cursor != "" {
		data, e := base64.RawURLEncoding.DecodeString(p.Cursor)
		if e != nil || json.Unmarshal(data, &cursor) != nil || cursor.Before == 0 {
			return fail("invalid cursor")
		}
		if cursor.Epoch != epoch || cursor.Generation != generation {
			return fail("cursor expired after eviction, clear or restart; restart search without cursor")
		}
		if cursor.Filters != fingerprint {
			return fail("cursor filters differ; repeat the original filters")
		}
	}
	result := SearchResult{Source: "memory", Requests: []RequestSummary{}, Limitations: []string{"Only currently retained exchanges are searched; no complete history is implied.", "Missing responses may be pending or unrecorded; network errors are not stored.", "Response and timing data may change between pages; new entries are excluded from subsequent pages.", "URL paths may contain sensitive data; query values, credentials and fragments are removed."}}
	if d.Capture != nil && !d.Capture.IsCapturing() {
		result.Limitations = append(result.Limitations, "Recording is currently paused.")
	}
	var last uint64
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, SearchResult{}, err
		}
		if cursor.Before != 0 && e.Sequence >= cursor.Before {
			continue
		}
		if !since.IsZero() && e.CreatedAt.Before(since) || !until.IsZero() && e.CreatedAt.After(until) {
			continue
		}
		if p.HasResponse != nil && *p.HasResponse != (e.Response != nil) {
			continue
		}
		if p.StatusCode != 0 && (e.Response == nil || int(e.Response.StatusCode) != p.StatusCode) {
			continue
		}
		u := requestURL(e.Request)
		if p.Host != "" && (u == nil || !strings.EqualFold(u.Hostname(), p.Host)) {
			continue
		}
		if p.PathContains != "" && (u == nil || !strings.Contains(u.Path, p.PathContains)) {
			continue
		}
		if p.Method != "" && (e.Request == nil || !strings.EqualFold(e.Request.Method, p.Method)) {
			continue
		}
		if len(result.Requests) == p.Limit {
			cursor.Before = last
			data, _ := json.Marshal(cursor)
			result.NextCursor = base64.RawURLEncoding.EncodeToString(data)
			break
		}
		result.Requests = append(result.Requests, summarizeRequest(e, u))
		last = e.Sequence
	}
	return nil, result, nil
}

// Handle absolute-form, origin-form with Host, and CONNECT authority-form.
// Invalid targets are omitted rather than echoed back without redaction.
func requestURL(r *storage.RequestRecord) *url.URL {
	if r == nil {
		return nil
	}
	target := r.URL
	if strings.EqualFold(r.Method, "CONNECT") {
		target = "//" + target
	}
	u, err := url.Parse(target)
	if err != nil {
		return nil
	}
	if u.Host == "" && strings.HasPrefix(target, "/") {
		for _, h := range r.Headers {
			if strings.EqualFold(h.Name, "Host") {
				host, err := url.Parse("//" + h.Value)
				if err == nil && host.Host != "" && host.Path == "" && host.RawQuery == "" && host.Fragment == "" {
					u.Host = host.Host
					u.User = host.User
				}
				break
			}
		}
	}
	if u.Opaque != "" || u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https" {
		return nil
	}
	return u
}

func summarizeRequest(e storage.CapturedExchange, u *url.URL) RequestSummary {
	s := RequestSummary{ID: e.CorrelationID, ObservedAt: e.CreatedAt, HasRequest: e.Request != nil, HasResponse: e.Response != nil, Visibility: "unknown"}
	if e.Request != nil {
		s.Method = e.Request.Method
		if strings.EqualFold(s.Method, "CONNECT") {
			s.Visibility = "connect"
		} else if u != nil && u.Scheme == "https" {
			s.Visibility = "https_request"
		} else if u != nil && u.Scheme == "http" {
			s.Visibility = "http_request"
		}
		if u == nil {
			s.URLRedacted = true
		} else {
			clean := *u
			s.URLRedacted = clean.User != nil || clean.RawQuery != "" || clean.Fragment != ""
			clean.User = nil
			clean.Fragment = ""
			clean.RawFragment = ""
			// Mask the entire query, including names: arbitrary keys can also be secrets.
			if clean.RawQuery != "" {
				clean.RawQuery = "REDACTED"
			}
			s.URL = clean.String()
			if len(s.URL) > 2048 {
				s.URL = string([]rune(s.URL)[:min(len([]rune(s.URL)), 512)]) + "…"
				s.URLTruncated = true
			}
		}
	}
	if e.Response != nil {
		code := e.Response.StatusCode
		s.StatusCode = &code
	}
	if e.Timing != nil {
		ms := float64(e.Timing.Total) / float64(time.Millisecond)
		s.DurationMS = &ms
	}
	return s
}
