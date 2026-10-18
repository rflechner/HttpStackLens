package mcp

import (
	"context"
	"fmt"
	"httpStackLens/storage"
	"net/url"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Bounds of what get_request returns for one side of an exchange: enough to
// diagnose, not enough to flood the model's context with one odd request.
const (
	maxDetailHeaders     = 100
	maxHeaderValueLength = 1024
)

// trafficRedacted stands in for a secret in captured traffic. It is not the
// composer's __REDACTED__: nothing is ever written back from it.
const trafficRedacted = "REDACTED"

type GetRequestParams struct {
	ID string `json:"id" jsonschema:"Exchange id, as returned by search_requests"`
}

type RequestDetail struct {
	UserAction  *UserAction   `json:"user_action,omitempty" jsonschema:"When present, guide the user through its steps: it cannot be done through MCP"`
	ID          string        `json:"id"`
	ObservedAt  time.Time     `json:"observed_at"`
	State       string        `json:"state" jsonschema:"complete (request and response recorded), no_response (pending, failed, or response not recorded) or response_only (request evicted or not recorded)"`
	Visibility  string        `json:"visibility" jsonschema:"connect (opaque tunnel; inner requests are separate exchanges when decrypted), https_request, http_request, or unknown"`
	Request     *RequestSide  `json:"request"`
	Response    *ResponseSide `json:"response"`
	Timing      *TimingDetail `json:"timing" jsonschema:"Measured phases; null when the exchange was not instrumented"`
	Limitations []string      `json:"limitations"`
}

type RequestSide struct {
	Method       string          `json:"method"`
	URL          string          `json:"url" jsonschema:"Userinfo and fragment removed, all query values redacted; paths may still contain sensitive data"`
	URLRedacted  bool            `json:"url_redacted"`
	URLTruncated bool            `json:"url_truncated"`
	HTTPVersion  string          `json:"http_version,omitempty"`
	Headers      []HeaderSummary `json:"headers"`
	HeadersCut   int             `json:"headers_omitted" jsonschema:"Headers left out beyond the first 100"`
	Body         BodyInfo        `json:"body"`
}

type ResponseSide struct {
	StatusCode  int16           `json:"status_code"`
	StatusText  string          `json:"status_text,omitempty"`
	HTTPVersion string          `json:"http_version,omitempty"`
	Headers     []HeaderSummary `json:"headers"`
	HeadersCut  int             `json:"headers_omitted" jsonschema:"Headers left out beyond the first 100"`
	Body        BodyInfo        `json:"body"`
}

type HeaderSummary struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Redacted  bool   `json:"redacted,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// BodyInfo describes a body without its content.
type BodyInfo struct {
	State       string `json:"state" jsonschema:"captured, empty, skipped (larger than the capture rule for its content type) or absent (no body, or not recorded)"`
	Size        int    `json:"size" jsonschema:"Captured bytes; 0 unless state is captured"`
	ContentType string `json:"content_type,omitempty"`
}

// TimingDetail is in milliseconds. A phase is null when it was not measured or
// did not happen, such as DNS and TLS on a reused connection: Composer sends
// only measure the total.
type TimingDetail struct {
	TotalMS    float64  `json:"total_ms"`
	DNSMS      *float64 `json:"dns_ms"`
	ConnectMS  *float64 `json:"connect_ms"`
	TLSMS      *float64 `json:"tls_ms"`
	TTFBMS     *float64 `json:"ttfb_ms" jsonschema:"Time to first response byte"`
	DownloadMS *float64 `json:"download_ms"`
}

func (d Dependencies) getRequest(_ context.Context, _ *sdk.CallToolRequest, p GetRequestParams) (*sdk.CallToolResult, RequestDetail, error) {
	if d.Requests == nil {
		return nil, RequestDetail{}, fmt.Errorf("request store unavailable")
	}
	id := strings.TrimSpace(p.ID)
	if id == "" || len(id) > 128 {
		return nil, RequestDetail{}, fmt.Errorf("id is required, as returned by search_requests")
	}
	e, ok := d.Requests.Get(id)
	if !ok {
		return nil, RequestDetail{}, fmt.Errorf("no exchange %q in memory: it was evicted (only the most recent exchanges are kept), cleared, or never recorded; search again", id)
	}
	detail := requestDetail(e)
	// Decryption only matters to an exchange it would have opened up.
	if detail.Visibility == "connect" {
		detail.UserAction = d.decryptionUserAction()
	}
	return nil, detail, nil
}

func requestDetail(e storage.CapturedExchange) RequestDetail {
	u := requestURL(e.Request)
	detail := RequestDetail{
		ID:          e.CorrelationID,
		ObservedAt:  e.CreatedAt,
		Visibility:  visibility(e.Request, u),
		Limitations: []string{"Header values that look like credentials are redacted; authentication headers keep their scheme only.", "Bodies are described, not returned."},
	}
	switch {
	case e.Request != nil && e.Response != nil:
		detail.State = "complete"
	case e.Request != nil:
		detail.State = "no_response"
		detail.Limitations = append(detail.Limitations, "No response recorded: the exchange may still be in progress or may have failed, and responses are only recorded for decrypted HTTPS and Composer sends — not for plain HTTP relayed by the proxy, nor for CONNECT tunnels. Network errors are not stored.")
	default:
		detail.State = "response_only"
		detail.Limitations = append(detail.Limitations, "The request side is missing: it was not recorded or was replaced.")
	}
	if detail.Visibility == "connect" {
		detail.Limitations = append(detail.Limitations, "CONNECT opens a tunnel: without HTTPS decryption, the requests inside it are not visible.")
	}

	if r := e.Request; r != nil {
		side := &RequestSide{Method: r.Method, HTTPVersion: httpVersion(r.HttpVersion)}
		side.URL, side.URLRedacted, side.URLTruncated = redactRequestURL(u)
		side.Headers, side.HeadersCut = summarizeHeaders(r.Headers)
		side.Body = bodyInfo(r.Body, r.BodySkipped, r.Headers)
		detail.Request = side
	}
	if r := e.Response; r != nil {
		side := &ResponseSide{StatusCode: r.StatusCode, StatusText: r.StatusMessage, HTTPVersion: httpVersion(r.HttpVersion)}
		side.Headers, side.HeadersCut = summarizeHeaders(r.Headers)
		side.Body = bodyInfo(r.Body, r.BodySkipped, r.Headers)
		detail.Response = side
		if r.StatusCode == 407 {
			detail.Limitations = append(detail.Limitations, "407 comes from a proxy (the upstream proxy, or HttpStackLens itself), not from the target server.")
		}
	}
	if t := e.Timing; t != nil {
		detail.Timing = &TimingDetail{
			TotalMS:    milliseconds(t.Total),
			DNSMS:      phase(t.Dns),
			ConnectMS:  phase(t.Connect),
			TLSMS:      phase(t.Tls),
			TTFBMS:     phase(t.Ttfb),
			DownloadMS: phase(t.Download),
		}
	} else {
		detail.Limitations = append(detail.Limitations, "No timing was measured for this exchange; durations must not be inferred.")
	}
	return detail
}

func summarizeHeaders(headers []storage.Header) ([]HeaderSummary, int) {
	out := make([]HeaderSummary, 0, min(len(headers), maxDetailHeaders))
	for i, h := range headers {
		if i == maxDetailHeaders {
			return out, len(headers) - maxDetailHeaders
		}
		out = append(out, redactTrafficHeader(h.Name, h.Value))
	}
	return out, 0
}

// redactTrafficHeader filters one captured header. What diagnosing a
// corporate proxy needs is kept: the authentication scheme (Negotiate, NTLM,
// Basic), cookie names and Set-Cookie attributes, the path of a redirect.
func redactTrafficHeader(name, value string) HeaderSummary {
	h := HeaderSummary{Name: name, Value: value}
	lower := strings.ToLower(name)
	switch {
	case lower == "cookie":
		h.Value, h.Redacted = redactCookies(value)
	case lower == "set-cookie":
		h.Value, h.Redacted = redactSetCookie(value)
	case strings.Contains(lower, "auth"):
		// Authorization, Proxy-Authorization, WWW-Authenticate, Proxy-Authenticate.
		h.Value, h.Redacted = keepScheme(value)
	case sensitiveName.MatchString(name):
		h.Value, h.Redacted = trafficRedacted, value != ""
	case lower == "location" || lower == "referer" || lower == "content-location":
		h.Value, h.Redacted = redactHeaderURL(value)
	}
	if len(h.Value) > maxHeaderValueLength {
		h.Value = truncateTo(h.Value, maxHeaderValueLength) + "…"
		h.Truncated = true
	}
	return h
}

// keepScheme keeps the first word of an authentication value, the scheme, and
// redacts the credentials or challenge after it. Several challenges in one
// header keep only the first scheme.
func keepScheme(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	scheme, rest, found := strings.Cut(value, " ")
	if !found || strings.ContainsAny(scheme, "=,") {
		// A bare token, or parameters without a scheme.
		if isAuthScheme(value) {
			return value, false
		}
		return trafficRedacted, true
	}
	if strings.TrimSpace(rest) == "" {
		return scheme, false
	}
	return scheme + " " + trafficRedacted, true
}

// isAuthScheme recognises a value that is only a scheme name, such as a
// bare "Negotiate" challenge.
func isAuthScheme(value string) bool {
	switch strings.ToLower(value) {
	case "basic", "bearer", "digest", "negotiate", "ntlm", "kerberos":
		return true
	}
	return false
}

func redactCookies(value string) (string, bool) {
	var names []string
	for _, part := range strings.Split(value, ";") {
		if name, _, _ := strings.Cut(strings.TrimSpace(part), "="); name != "" {
			names = append(names, name+"="+trafficRedacted)
		}
	}
	return strings.Join(names, "; "), value != ""
}

// redactSetCookie hides the value of the cookie and keeps its attributes,
// which tell why a browser would not send it back.
func redactSetCookie(value string) (string, bool) {
	first, attributes, _ := strings.Cut(value, ";")
	name, _, _ := strings.Cut(strings.TrimSpace(first), "=")
	out := name + "=" + trafficRedacted
	if attributes != "" {
		out += ";" + attributes
	}
	return out, true
}

// redactHeaderURL masks the query and userinfo of a URL-valued header, as for
// the request URL. A relative reference keeps its path.
func redactHeaderURL(value string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return trafficRedacted, true
	}
	if u.User == nil && u.RawQuery == "" && u.Fragment == "" {
		return value, false
	}
	text, _, _ := redactRequestURL(u)
	return text, true
}

// bodyInfo needs the headers only for the content type.
func bodyInfo(body []byte, skipped bool, headers []storage.Header) BodyInfo {
	info := BodyInfo{State: "absent"}
	for _, h := range headers {
		if strings.EqualFold(h.Name, "Content-Type") {
			info.ContentType = h.Value
			break
		}
	}
	switch {
	case skipped:
		info.State = "skipped"
	case body == nil:
	case len(body) == 0:
		info.State = "empty"
	default:
		info.State = "captured"
		info.Size = len(body)
	}
	return info
}

func httpVersion(v storage.HttpVersion) string {
	if v == storage.HttpVersionUnknown {
		return ""
	}
	return fmt.Sprintf("HTTP/%d.%d", v.Major(), v.Minor())
}

func milliseconds(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

func phase(d time.Duration) *float64 {
	if d <= 0 {
		return nil
	}
	ms := milliseconds(d)
	return &ms
}
