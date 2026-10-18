package configuration

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// PersistMCPConfig writes the mcp section back to config.yaml, so that a
// listener started, stopped or reconfigured from the Web UI comes back the same
// way on the next start.
func PersistMCPConfig(config MCPConfig) error {
	return persistMCPConfig(ResolveConfigPath(), config)
}

// persistMCPConfig updates each key of the mcp section in place, keeping its
// trailing comment. A key missing from the section is added at its end, and a
// missing section is added at the end of the file.
func persistMCPConfig(path string, config MCPConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	values := [][2]string{
		{"enabled", strconv.FormatBool(config.Enabled)},
		{"port", strconv.Itoa(config.Port)},
		{"require_token", strconv.FormatBool(config.RequireToken)},
		{"token", strconv.Quote(config.Token)},
	}
	lines := strings.SplitAfter(string(data), "\n")
	lines = setTopLevelSectionKeys(lines, "mcp", values)
	return os.WriteFile(path, []byte(strings.Join(lines, "")), 0o644)
}

// setTopLevelSectionKeys sets scalar keys directly under a top-level section.
// It only rewrites the lines of those keys: the rest of the section, comments
// included, is left as written.
func setTopLevelSectionKeys(lines []string, section string, values [][2]string) []string {
	start, end := findTopLevelSection(lines, section)
	if start < 0 {
		if n := len(lines); n > 0 && !strings.HasSuffix(lines[n-1], "\n") {
			lines[n-1] += "\n"
		}
		lines = append(lines, section+":\n")
		for _, kv := range values {
			lines = append(lines, fmt.Sprintf("  %s: %s\n", kv[0], kv[1]))
		}
		return lines
	}

	// Children use the indentation of the first one; an empty section gets two
	// spaces, like the rest of the generated file.
	childIndent := 2
	insertAt := start + 1
	for i := start + 1; i < end; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if insertAt == start+1 {
			childIndent = leadingSpaces(lines[i])
		}
		insertAt = i + 1
	}
	prefix := strings.Repeat(" ", childIndent)

	var missing []string
	for _, kv := range values {
		key, value := kv[0], kv[1]
		found := false
		for i := start + 1; i < end; i++ {
			line := lines[i]
			if leadingSpaces(line) != childIndent {
				continue
			}
			name, rest, ok := strings.Cut(strings.TrimLeft(line, " "), ":")
			if !ok || strings.TrimSpace(name) != key {
				continue
			}
			lines[i] = prefix + key + ": " + value + trailingComment(rest) + lineEnding(line)
			found = true
			break
		}
		if !found {
			missing = append(missing, prefix+key+": "+value+"\n")
		}
	}
	if len(missing) == 0 {
		return lines
	}
	if insertAt > 0 && !strings.HasSuffix(lines[insertAt-1], "\n") {
		lines[insertAt-1] += "\n"
	}
	out := make([]string, 0, len(lines)+len(missing))
	out = append(out, lines[:insertAt]...)
	out = append(out, missing...)
	out = append(out, lines[insertAt:]...)
	return out
}

// findTopLevelSection returns the line of "section:" and the index of the first
// line past its body, or (-1, -1) when the file has no such section.
func findTopLevelSection(lines []string, section string) (start, end int) {
	start = -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaces(line)
		if start >= 0 && indent == 0 {
			return start, i
		}
		if indent == 0 && withoutComment(trimmed) == section+":" {
			start = i
		}
	}
	if start < 0 {
		return -1, -1
	}
	return start, len(lines)
}

// withoutComment drops a trailing "# …" comment. It does not try to recognise
// a # inside a quoted value: the keys it is used on hold booleans, numbers and
// hex tokens.
func withoutComment(s string) string {
	if idx := strings.Index(s, "#"); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return s
}

// trailingComment returns " # …" from the value part of a line, or "".
func trailingComment(rest string) string {
	if idx := strings.Index(rest, "#"); idx >= 0 {
		return " " + strings.TrimRight(rest[idx:], "\r\n")
	}
	return ""
}

func lineEnding(line string) string {
	switch {
	case strings.HasSuffix(line, "\r\n"):
		return "\r\n"
	case strings.HasSuffix(line, "\n"):
		return "\n"
	}
	return ""
}
