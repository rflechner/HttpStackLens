//go:build js && wasm

package composer

import (
	"strings"
	"testing"
)

// roundTrip is what the "Edit as" switch does: the raw text is parsed into the
// model and the model is written back out. Anything the pair adds on the way
// shows up as a difference here.
func roundTrip(t *testing.T, source string) string {
	t.Helper()
	return ToHTTP(ParseHTTP(source, "requests.http"))
}

// assertStable checks the property the composer depends on: switching between
// the form and the raw editor any number of times must leave the file alone.
func assertStable(t *testing.T, source string) {
	t.Helper()
	first := roundTrip(t, source)
	if first != source {
		t.Errorf("round-trip changed the file:\n--- before ---\n%s\n--- after ---\n%s", source, first)
	}
	for i := 2; i <= 4; i++ {
		next := roundTrip(t, first)
		if next != first {
			t.Errorf("round-trip %d changed the file again:\n--- before ---\n%s\n--- after ---\n%s", i, first, next)
			return
		}
	}
}

func TestConsecutiveSeparatorsAreOneRequest(t *testing.T) {
	source := "### GET request example for calling API of ipify.org\n" +
		"### enforcing IPv4\n" +
		"GET https://api.ipify.org?format=json\n" +
		"Accept: application/json\n"

	file := ParseHTTP(source, "requests.http")

	if len(file.Reqs) != 1 {
		names := make([]string, 0, len(file.Reqs))
		for _, r := range file.Reqs {
			names = append(names, r.Title())
		}
		t.Fatalf("want 1 request, got %d: %s", len(file.Reqs), strings.Join(names, " | "))
	}
	r := file.Reqs[0]
	if r.Name != "GET request example for calling API of ipify.org" {
		t.Errorf("name = %q", r.Name)
	}
	if want := []string{"### enforcing IPv4"}; len(r.Notes) != 1 || r.Notes[0] != want[0] {
		t.Errorf("notes = %q, want %q", r.Notes, want)
	}
	if r.Method != "GET" || r.URL != "https://api.ipify.org?format=json" {
		t.Errorf("request line = %q %q", r.Method, r.URL)
	}
	assertStable(t, source)
}

// The sample shipped with the parser is the file the bug was reported on: it
// opens on two separators and closes on a bare one.
func TestSampleFileIsStable(t *testing.T) {
	assertStable(t, "### GET request example for calling API of ipify.org\n"+
		"### enforcing IPv4\n"+
		"GET https://api.ipify.org?format=json\n"+
		"Accept: application/json\n"+
		"\n"+
		"### Accept IPv4 or IPv6\n"+
		"GET https://api64.ipify.org?format=json\n"+
		"Accept: application/json\n"+
		"\n"+
		"###\n")
}

// A block with nothing but its separator must not grow a request line. This is
// what used to turn a trailing "###" into "### Untitled\nGET \n", then into
// "GET GET" on the pass after that.
func TestEmptyBlockKeepsToItsTitle(t *testing.T) {
	file := ParseHTTP("###\n", "requests.http")
	if len(file.Reqs) != 1 {
		t.Fatalf("want 1 request, got %d", len(file.Reqs))
	}
	if got := file.Reqs[0].Title(); got != "Untitled" {
		t.Errorf("Title() = %q, want the display placeholder", got)
	}
	if name := file.Reqs[0].Name; name != "" {
		t.Errorf("Name = %q, want the placeholder to stay out of the model", name)
	}
	assertStable(t, "###\n")
}

// A verb with no target is the request line of a block being written — the
// state "+ New request" leaves the file in once its headers are cleared.
func TestVerbWithoutTarget(t *testing.T) {
	file := ParseHTTP("### Draft\nPOST\n", "requests.http")
	r := file.Reqs[0]
	if r.Method != "POST" || r.URL != "" {
		t.Fatalf("request line = %q %q, want the verb read as a method", r.Method, r.URL)
	}
	assertStable(t, "### Draft\nPOST\nAccept: application/json\n")
}

func TestCommentsAboveTheRequestLineSurvive(t *testing.T) {
	assertStable(t, "### Current user\n"+
		"# needs the token below\n"+
		"// @no-cookie-jar\n"+
		"GET https://api.github.com/user\n"+
		"Accept: application/vnd.github+json\n")
}

// The raw editor addresses lines through the textarea's selection, counted in
// UTF-16 code units. A body holding an em dash or an emoji is where a byte
// offset would put the caret on the wrong line.
func TestUTF16LineConversions(t *testing.T) {
	const text = "### Créé — accentué\nGET https://x/é\n\n{\"k\":\"🙂\"}\nlast"

	// Line 1 is 19 units wide, line 2 is 15 (the é counts once), line 3 empty,
	// line 4 is 10 (the emoji is a surrogate pair, so it counts twice).
	for line, want := range map[int]int{1: 0, 2: 20, 3: 36, 4: 37, 5: 48} {
		if got := utf16LineOffset(text, line); got != want {
			t.Errorf("utf16LineOffset(line %d) = %d, want %d", line, got, want)
		}
		if got := utf16LineAt(text, want); got != line {
			t.Errorf("utf16LineAt(offset %d) = %d, want line %d", want, got, line)
		}
	}

	// A line past the end clamps to the end rather than reaching out of range.
	if got, want := utf16LineOffset(text, 99), 48+len("last"); got != want {
		t.Errorf("utf16LineOffset past the end = %d, want %d", got, want)
	}
	// A caret in the middle of a line stays on that line.
	if got := utf16LineAt(text, 25); got != 2 {
		t.Errorf("utf16LineAt(mid-line) = %d, want 2", got)
	}
}

func TestVariablesBodyAndSeveralRequestsAreStable(t *testing.T) {
	assertStable(t, "@baseUrl = https://api.github.com\n"+
		"@token = ghp_exampletoken\n"+
		"\n"+
		"### Current user\n"+
		"GET {{baseUrl}}/user\n"+
		"Authorization: Bearer {{token}}\n"+
		"\n"+
		"### Create issue comment\n"+
		"POST {{baseUrl}}/repos/golang/go/issues/68412/comments\n"+
		"Content-Type: application/json\n"+
		"\n"+
		"{\n"+
		"  \"body\": \"Reproduced on go1.23.2.\"\n"+
		"}\n")
}

// FileName mirrors what the backend accepts, so that the name field can refuse
// a bad name without a round trip. These are the cases the two have to agree on.
func TestFileNameNormalises(t *testing.T) {
	cases := map[string]string{
		"github":        "github.http",
		"  corp-auth  ": "corp-auth.http",
		"orders.http":   "orders.http",
		"orders.HTTP":   "orders.HTTP",
		"notes.txt":     "notes.txt.http",
	}
	for input, want := range cases {
		got, err := FileName(input)
		if err != nil {
			t.Errorf("FileName(%q): %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("FileName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFileNameRefusesPathsAndReservedCharacters(t *testing.T) {
	for _, input := range []string{"", "   ", "..", "../escape", "nested/file", `nested\file`, ".hidden", "pipe|name", "star*"} {
		if got, err := FileName(input); err == nil {
			t.Errorf("expected %q to be refused, got %q", input, got)
		}
	}
}
