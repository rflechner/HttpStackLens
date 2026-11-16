package configuration

import (
	"fmt"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
)

func PersistDesktopStartup(enabled bool) error {
	configPersistenceMu.Lock()
	defer configPersistenceMu.Unlock()
	return persistDesktopStartup(ResolveConfigPath(), enabled)
}

// Preserve the rest of the YAML, including comments and unrelated settings.
func persistDesktopStartup(path string, enabled bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.SplitAfter(string(data), "\n")
	start, end := -1, len(lines)
	for i, line := range lines {
		text := strings.TrimSpace(line)
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if start >= 0 && leadingSpaces(line) == 0 {
			end = i
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if leadingSpaces(line) == 0 && ok && strings.TrimSpace(key) == "desktop" {
			value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
			if value != "" {
				var section map[string]map[string]any
				if err := yaml.Unmarshal([]byte(line), &section); err != nil {
					return fmt.Errorf("read desktop settings: %w", err)
				}
				if section["desktop"] == nil {
					section["desktop"] = make(map[string]any)
				}
				section["desktop"]["start_at_login"] = enabled
				block, err := yaml.Marshal(section)
				if err != nil {
					return err
				}
				lines[i] = string(block)
				return os.WriteFile(path, []byte(strings.Join(lines, "")), 0o644)
			}
			start = i
		}
	}
	valueLine := fmt.Sprintf("  start_at_login: %t\n", enabled)
	if start < 0 {
		text := string(data)
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		return os.WriteFile(path, []byte(text+"desktop:\n"+valueLine), 0o644)
	}
	for i := start + 1; i < end; i++ {
		key, rest, ok := strings.Cut(strings.TrimSpace(lines[i]), ":")
		if leadingSpaces(lines[i]) != 2 || !ok || key != "start_at_login" {
			continue
		}
		if index := strings.Index(rest, "#"); index >= 0 {
			valueLine = strings.TrimSuffix(valueLine, "\n") + " " + strings.TrimSpace(rest[index:]) + "\n"
		}
		lines[i] = valueLine
		return os.WriteFile(path, []byte(strings.Join(lines, "")), 0o644)
	}
	prefix := strings.Join(lines[:end], "")
	if !strings.HasSuffix(prefix, "\n") {
		prefix += "\n"
	}
	return os.WriteFile(path, []byte(prefix+valueLine+strings.Join(lines[end:], "")), 0o644)
}
