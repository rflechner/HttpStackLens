package middlewares

import (
	"httpStackLens/http/models"
	"log/slog"
	"net"
	"strings"
)

// NoProxyRouter decides, per request, whether the target host should be
// forwarded to the upstream proxy or connected to directly. Hosts matching one
// of the Rules (a no_proxy list) are handed to Direct — connecting straight to
// the origin instead of forwarding to the upstream; everything else flows to
// Upstream (the upstream forwarder).
//
// It only changes behaviour when an upstream proxy is configured; it does not
// affect HTTPS decryption, which is handled upstream of this router.
type NoProxyRouter struct {
	Rules    []string   // no_proxy patterns (host, .suffix, or CIDR)
	Upstream Middleware // used when the host does NOT match (forward to the upstream)
	Direct   Middleware // used when the host matches (connect directly to the origin)
}

func (m *NoProxyRouter) HandleProxyRequest(browser net.Conn, request models.ProxyRequest) error {
	host := request.HttpRequestLine.Endpoint.Host
	if MatchesNoProxy(host, m.Rules) {
		slog.Debug("no_proxy: routing directly (bypassing upstream)",
			"host", host, "method", string(request.HttpRequestLine.HttpMethod))
		return m.Direct.HandleProxyRequest(browser, request)
	}
	slog.Debug("no_proxy: forwarding to upstream",
		"host", host, "method", string(request.HttpRequestLine.HttpMethod))
	return m.Upstream.HandleProxyRequest(browser, request)
}

// MatchesNoProxy reports whether host matches one of the no_proxy rules. The
// matching mirrors the common NO_PROXY environment-variable semantics:
//
//   - "*" matches every host.
//   - a leading-dot rule (".local") matches the domain itself ("local") and any
//     subdomain ("foo.local").
//   - a plain rule ("example.com") matches the host exactly and any subdomain
//     ("api.example.com").
//   - a CIDR rule ("10.0.0.0/8") matches when the host is an IP inside it.
//
// Matching is case-insensitive and ignores a trailing dot on the host.
func MatchesNoProxy(host string, rules []string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" {
		return false
	}
	hostIP := net.ParseIP(host)

	for _, rule := range rules {
		rule = strings.ToLower(strings.TrimSpace(rule))
		if rule == "" {
			continue
		}
		if rule == "*" {
			return true
		}

		// CIDR rule: match when the host is an IP within the range.
		if hostIP != nil && strings.Contains(rule, "/") {
			if _, network, err := net.ParseCIDR(rule); err == nil && network.Contains(hostIP) {
				return true
			}
			continue
		}

		if strings.HasPrefix(rule, ".") {
			if host == rule[1:] || strings.HasSuffix(host, rule) {
				return true
			}
			continue
		}

		if host == rule || strings.HasSuffix(host, "."+rule) {
			return true
		}
	}
	return false
}
