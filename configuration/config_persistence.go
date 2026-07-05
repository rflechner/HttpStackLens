package configuration

import (
	"fmt"
	"os"
	"strings"
)

const defaultConfigPath = "config.yaml"

// PersistStorageEnabled updates storage.enable in config.yaml while preserving
// the rest of the file as-is. The Web UI uses this for pause/resume so capture
// preference survives the next application start.
func PersistStorageEnabled(enabled bool) error {
	return persistStorageEnabled(defaultConfigPath, enabled)
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
