package configuration

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const defaultConfigPath = "config.yaml"

// PersistStorageEnabled updates storage.enable in config.yaml while preserving
// the rest of the file as-is. The Web UI uses this for pause/resume so capture
// preference survives the next application start.
func PersistStorageEnabled(enabled bool) error {
	return persistStorageEnabled(defaultConfigPath, enabled)
}

func PersistDecryptHttpsCaptureRules(config DecryptHttpsConfig) error {
	return persistDecryptHttpsCaptureRules(defaultConfigPath, config)
}

// PersistUpstreamSettings writes the upstream proxy settings (output_proxy_uri,
// add_windows_authentication_to_output_proxy, no_proxy) back into the proxy
// section of config.yaml so Web UI edits survive the next application start.
func PersistUpstreamSettings(settings UpstreamSettings) error {
	return persistUpstreamSettings(defaultConfigPath, settings)
}

func persistStorageEnabled(path string, enabled bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	newValue := "false"
	if enabled {
		newValue = "true"
	}

	lines := strings.SplitAfter(string(data), "\n")
	inStorage := false
	storageIndent := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := leadingSpaces(line)
		if inStorage && indent <= storageIndent {
			break
		}
		if !inStorage {
			if trimmed == "storage:" {
				inStorage = true
				storageIndent = indent
			}
			continue
		}

		key, rest, ok := strings.Cut(strings.TrimLeft(line, " "), ":")
		if !ok || strings.TrimSpace(key) != "enable" {
			continue
		}

		comment := ""
		if idx := strings.Index(rest, "#"); idx >= 0 {
			comment = strings.TrimRight(rest[idx:], "\r\n")
		}
		lineEnd := ""
		if strings.HasSuffix(line, "\n") {
			lineEnd = "\n"
		}
		prefix := line[:indent]
		if comment != "" {
			lines[i] = fmt.Sprintf("%senable: %s %s%s", prefix, newValue, comment, lineEnd)
		} else {
			lines[i] = fmt.Sprintf("%senable: %s%s", prefix, newValue, lineEnd)
		}
		return os.WriteFile(path, []byte(strings.Join(lines, "")), 0o644)
	}

	return fmt.Errorf("configuration: storage.enable not found in %s", path)
}

func leadingSpaces(s string) int {
	n := 0
	for n < len(s) && s[n] == ' ' {
		n++
	}
	return n
}

func persistDecryptHttpsCaptureRules(path string, config DecryptHttpsConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.SplitAfter(string(data), "\n")
	sectionStart := -1
	sectionEnd := len(lines)
	sectionIndent := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaces(line)
		if trimmed == "decrypt_https:" {
			sectionStart = i
			sectionIndent = indent
			continue
		}
		if sectionStart >= 0 && indent <= sectionIndent {
			sectionEnd = i
			break
		}
	}
	if sectionStart < 0 {
		return fmt.Errorf("configuration: decrypt_https not found in %s", path)
	}

	out := make([]string, 0, len(lines)+len(config.MimeTypes)*3+2)
	out = append(out, lines[:sectionStart+1]...)
	for i := sectionStart + 1; i < sectionEnd; i++ {
		if shouldRemoveCaptureRuleLine(lines, i, sectionIndent) {
			i = skipYamlBlock(lines, i, sectionEnd)
			continue
		}
		out = append(out, lines[i])
	}
	out = append(out, renderCaptureRuleLines(sectionIndent+2, config)...)
	out = append(out, lines[sectionEnd:]...)

	return os.WriteFile(path, []byte(strings.Join(out, "")), 0o644)
}

func shouldRemoveCaptureRuleLine(lines []string, i, parentIndent int) bool {
	line := lines[i]
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false
	}
	indent := leadingSpaces(line)
	if indent != parentIndent+2 {
		return false
	}
	key, _, ok := strings.Cut(strings.TrimLeft(line, " "), ":")
	if !ok {
		return false
	}
	key = strings.TrimSpace(key)
	return key == "default_max_bytes" || key == "mime_types"
}

func skipYamlBlock(lines []string, start, sectionEnd int) int {
	indent := leadingSpaces(lines[start])
	for i := start + 1; i < sectionEnd; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if leadingSpaces(lines[i]) <= indent {
			return i - 1
		}
	}
	return sectionEnd - 1
}

func persistUpstreamSettings(path string, settings UpstreamSettings) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.SplitAfter(string(data), "\n")
	sectionStart := -1
	sectionEnd := len(lines)
	sectionIndent := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaces(line)
		if trimmed == "proxy:" {
			sectionStart = i
			sectionIndent = indent
			continue
		}
		if sectionStart >= 0 && indent <= sectionIndent {
			sectionEnd = i
			break
		}
	}
	if sectionStart < 0 {
		return fmt.Errorf("configuration: proxy not found in %s", path)
	}

	out := make([]string, 0, len(lines)+len(settings.NoProxy)+4)
	out = append(out, lines[:sectionStart+1]...)
	for i := sectionStart + 1; i < sectionEnd; i++ {
		if shouldRemoveUpstreamLine(lines, i, sectionIndent) {
			i = skipYamlBlock(lines, i, sectionEnd)
			continue
		}
		out = append(out, lines[i])
	}
	out = append(out, renderUpstreamLines(sectionIndent+2, settings)...)
	out = append(out, lines[sectionEnd:]...)

	return os.WriteFile(path, []byte(strings.Join(out, "")), 0o644)
}

func shouldRemoveUpstreamLine(lines []string, i, parentIndent int) bool {
	line := lines[i]
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false
	}
	if leadingSpaces(line) != parentIndent+2 {
		return false
	}
	key, _, ok := strings.Cut(strings.TrimLeft(line, " "), ":")
	if !ok {
		return false
	}
	switch strings.TrimSpace(key) {
	case "output_proxy_uri", "add_windows_authentication_to_output_proxy", "no_proxy":
		return true
	default:
		return false
	}
}

func renderUpstreamLines(indent int, settings UpstreamSettings) []string {
	prefix := strings.Repeat(" ", indent)
	lines := make([]string, 0, len(settings.NoProxy)+3)

	uri := strings.TrimSpace(settings.OutputProxyUri)
	if uri == "" {
		lines = append(lines, fmt.Sprintf("%soutput_proxy_uri:\n", prefix))
	} else {
		lines = append(lines, fmt.Sprintf("%soutput_proxy_uri: %s\n", prefix, strconv.Quote(uri)))
	}

	lines = append(lines, fmt.Sprintf("%sadd_windows_authentication_to_output_proxy: %t\n", prefix, settings.AddWindowsAuthentication))

	if len(settings.NoProxy) == 0 {
		lines = append(lines, fmt.Sprintf("%sno_proxy: []\n", prefix))
	} else {
		lines = append(lines, fmt.Sprintf("%sno_proxy:\n", prefix))
		for _, host := range settings.NoProxy {
			lines = append(lines, fmt.Sprintf("%s  - %s\n", prefix, strconv.Quote(host)))
		}
	}
	return lines
}

func renderCaptureRuleLines(indent int, config DecryptHttpsConfig) []string {
	prefix := strings.Repeat(" ", indent)
	lines := make([]string, 0, len(config.MimeTypes)*3+2)
	if config.DefaultMaxBytes != nil {
		lines = append(lines, fmt.Sprintf("%sdefault_max_bytes: %d\n", prefix, *config.DefaultMaxBytes))
	}
	lines = append(lines, fmt.Sprintf("%smime_types:\n", prefix))
	for _, rule := range config.MimeTypes {
		lines = append(lines, fmt.Sprintf("%s  - name: %s\n", prefix, strconv.Quote(rule.Name)))
		if rule.MaxSizeBytes != nil {
			lines = append(lines, fmt.Sprintf("%s    max_size_bytes: %d\n", prefix, *rule.MaxSizeBytes))
		} else if rule.MaxSizeKb != nil {
			lines = append(lines, fmt.Sprintf("%s    max_size_kb: %s\n", prefix, strconv.FormatFloat(*rule.MaxSizeKb, 'f', -1, 64)))
		} else if rule.MaxSizeMb != nil {
			lines = append(lines, fmt.Sprintf("%s    max_size_mb: %s\n", prefix, strconv.FormatFloat(*rule.MaxSizeMb, 'f', -1, 64)))
		}
	}
	return lines
}
