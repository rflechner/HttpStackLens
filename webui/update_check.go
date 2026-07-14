package webui

import (
	"encoding/json"
	"fmt"
	"httpStackLens/webui/wasm/shared"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// updateCheckTTL is how long a GitHub /releases/latest result is cached. The
// running release never changes, and the check is best-effort, so a long TTL
// keeps us well within GitHub's unauthenticated rate limit (60 req/h per IP).
const updateCheckTTL = time.Hour

// githubRelease is the subset of the GitHub release payload we consume.
type githubRelease struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// updateChecker queries GitHub for the latest release and caches the comparison
// against the running version. It is safe for concurrent use.
type updateChecker struct {
	currentVersion string
	apiURL         string
	userAgent      string
	client         *http.Client

	mu        sync.Mutex
	cached    shared.UpdateCheckDto
	fetchedAt time.Time
}

// newUpdateChecker builds a checker for the given "owner/name" repo. A generic,
// project-scoped User-Agent identifies the client to GitHub as requested by
// their API guidelines.
func newUpdateChecker(currentVersion, repo string) *updateChecker {
	return &updateChecker{
		currentVersion: currentVersion,
		apiURL:         fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo),
		userAgent:      "update-checker/" + currentVersion,
		client:         &http.Client{Timeout: 8 * time.Second},
	}
}

// result returns the current update status, refreshing from GitHub when the
// cache is stale. Dev builds (no valid version) short-circuit without a network
// call and report Checked=false.
func (u *updateChecker) result() shared.UpdateCheckDto {
	u.mu.Lock()
	defer u.mu.Unlock()

	if _, _, ok := parseSemver(u.currentVersion); !ok {
		return shared.UpdateCheckDto{Checked: false, CurrentVersion: u.currentVersion}
	}
	if !u.fetchedAt.IsZero() && time.Since(u.fetchedAt) < updateCheckTTL {
		return u.cached
	}

	dto := u.fetch()
	// Only cache successful checks so a transient GitHub/network failure is
	// retried on the next request rather than pinned for a full TTL.
	if dto.Checked {
		u.cached = dto
		u.fetchedAt = time.Now()
	}
	return dto
}

// fetch performs the actual GitHub request and comparison. Any failure yields a
// Checked=false result (the UI shows no badge) rather than an error surfaced to
// the user — update checking is non-essential.
func (u *updateChecker) fetch() shared.UpdateCheckDto {
	fail := shared.UpdateCheckDto{Checked: false, CurrentVersion: u.currentVersion}

	req, err := http.NewRequest(http.MethodGet, u.apiURL, nil)
	if err != nil {
		return fail
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", u.userAgent)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := u.client.Do(req)
	if err != nil {
		return fail
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fail
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fail
	}
	if release.Draft || release.Prerelease || !isValidSemver(release.TagName) {
		return fail
	}

	return shared.UpdateCheckDto{
		Checked:         true,
		CurrentVersion:  u.currentVersion,
		LatestVersion:   release.TagName,
		UpdateAvailable: isNewer(release.TagName, u.currentVersion),
		ReleaseURL:      release.HTMLURL,
		AssetURL:        selectAsset(release.Assets),
	}
}

// selectAsset returns the download URL whose name matches the current OS/arch
// (e.g. "darwin_arm64", "windows_amd64"), or "" when none is published for this
// platform — the UI then links to the release page instead.
func selectAsset(assets []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}) string {
	token := runtime.GOOS + "_" + runtime.GOARCH
	for _, a := range assets {
		if strings.Contains(a.Name, token) {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// updateCheckHandler serves the cached update status.
func updateCheckHandler(checker *updateChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, checker.result())
	}
}

// --- SemVer helpers (vMAJOR.MINOR.PATCH[-prerelease][+build]) ---

// parseSemver extracts the numeric core and reports whether a pre-release
// suffix is present. ok is false for anything that isn't a clean X.Y.Z core
// (including "dev", "unknown", or an empty string).
func parseSemver(v string) (core [3]int, prerelease bool, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexByte(v, '+'); i >= 0 { // strip build metadata
		v = v[:i]
	}
	base := v
	if i := strings.IndexByte(v, '-'); i >= 0 { // split off pre-release
		base = v[:i]
		prerelease = true
	}
	parts := strings.Split(base, ".")
	if len(parts) != 3 {
		return core, prerelease, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return core, prerelease, false
		}
		core[i] = n
	}
	return core, prerelease, true
}

func isValidSemver(v string) bool {
	_, _, ok := parseSemver(v)
	return ok
}

// isNewer reports whether latest is strictly newer than current. When both
// share the same numeric core, a final release outranks a pre-release of that
// core (so a between-tags build like v0.2.0-3-gabc updates to the real v0.2.0).
func isNewer(latest, current string) bool {
	ln, lpre, lok := parseSemver(latest)
	cn, cpre, cok := parseSemver(current)
	if !lok || !cok {
		return false
	}
	for i := 0; i < 3; i++ {
		if ln[i] != cn[i] {
			return ln[i] > cn[i]
		}
	}
	return cpre && !lpre
}
