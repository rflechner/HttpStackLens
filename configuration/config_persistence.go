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

// PersistDecryptHttpsEnabled updates decrypt_https.enabled in config.yaml while
// preserving certificate and body capture settings.
func PersistDecryptHttpsEnabled(enabled bool) error {
	return persistDecryptHttpsEnabled(defaultConfigPath, enabled)
}

// PersistUpstreamSettings writes the upstream proxy settings (output_proxy_uri,
// add_windows_authentication_to_output_proxy, no_proxy) back into the proxy
// section of config.yaml so Web UI edits survive the next application start.
func PersistUpstreamSettings(settings UpstreamSettings) error {
	return persistUpstreamSettings(defaultConfigPath, settings)
}

// PersistAccessControlSettings writes the new access_control blocks back to
// config.yaml and removes the legacy enable_remote_connection keys from the
// proxy/webui sections.
func PersistAccessControlSettings(settings AccessControlSettings) error {
	return persistAccessControlSettings(defaultConfigPath, settings)
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

func persistDecryptHttpsEnabled(path string, enabled bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.SplitAfter(string(data), "\n")
	inDecryptHttps := false
	sectionIndent := 0
	value := "false"
	if enabled {
		value = "true"
	}
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := leadingSpaces(line)
		if inDecryptHttps && indent <= sectionIndent {
			break
		}
		if !inDecryptHttps {
			if trimmed == "decrypt_https:" {
				inDecryptHttps = true
				sectionIndent = indent
			}
			continue
		}
		if indent != sectionIndent+2 {
			continue
		}

		key, rest, ok := strings.Cut(strings.TrimLeft(line, " "), ":")
		if !ok || strings.TrimSpace(key) != "enabled" {
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
			lines[i] = fmt.Sprintf("%senabled: %s %s%s", prefix, value, comment, lineEnd)
		} else {
			lines[i] = fmt.Sprintf("%senabled: %s%s", prefix, value, lineEnd)
		}
		return os.WriteFile(path, []byte(strings.Join(lines, "")), 0o644)
	}

	return fmt.Errorf("configuration: decrypt_https.enabled not found in %s", path)
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

func persistAccessControlSettings(path string, settings AccessControlSettings) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.SplitAfter(string(data), "\n")
	lines, err = replaceAccessControlSection(lines, "proxy", settings.Proxy)
	if err != nil {
		return err
	}
	lines, err = replaceAccessControlSection(lines, "webui", settings.WebUi)
	if err != nil {
		return err
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "")), 0o644)
}

func replaceAccessControlSection(lines []string, section string, config AccessControlConfig) ([]string, error) {
	sectionStart := -1
	sectionEnd := len(lines)
	sectionIndent := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaces(line)
		if trimmed == section+":" {
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
		return nil, fmt.Errorf("configuration: %s not found", section)
	}

	out := make([]string, 0, len(lines)+len(config.Networks)+3)
	out = append(out, lines[:sectionStart+1]...)
	for i := sectionStart + 1; i < sectionEnd; i++ {
		if shouldRemoveAccessControlLine(lines, i, sectionIndent) {
			i = skipYamlBlock(lines, i, sectionEnd)
			continue
		}
		out = append(out, lines[i])
	}
	out = append(out, renderAccessControlLines(sectionIndent+2, config)...)
	out = append(out, lines[sectionEnd:]...)
	return out, nil
}

func shouldRemoveAccessControlLine(lines []string, i, parentIndent int) bool {
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
	case "enable_remote_connection", "access_control":
		return true
	default:
		return false
	}
}

func renderAccessControlLines(indent int, config AccessControlConfig) []string {
	prefix := strings.Repeat(" ", indent)
	lines := []string{
		fmt.Sprintf("%saccess_control:\n", prefix),
		fmt.Sprintf("%s  mode: %s\n", prefix, strconv.Quote(string(config.Mode))),
	}
	if len(config.Networks) == 0 {
		lines = append(lines, fmt.Sprintf("%s  networks: []\n", prefix))
	} else {
		lines = append(lines, fmt.Sprintf("%s  networks:\n", prefix))
		for _, network := range config.Networks {
			lines = append(lines, fmt.Sprintf("%s    - %s\n", prefix, strconv.Quote(network)))
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
