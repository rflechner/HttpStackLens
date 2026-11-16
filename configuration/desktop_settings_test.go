package configuration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestPersistDesktopStartupRoundTrip(t *testing.T) {
	for _, input := range []string{
		"# keep\nproxy:\n  port: 3128\n",
		"# keep\nproxy:\n  port: 3128", // no final newline
		"# keep\ndesktop:\n  start_at_login: false # startup choice\nproxy:\n  port: 3128\n",
		"# keep\ndesktop:\n  future_setting: yes\nproxy:\n  port: 3128\n",
		"# keep\ndesktop: {}\nproxy:\n  port: 3128\n",
		"# keep\ndesktop: {start_at_login: false, future_setting: yes}\nproxy:\n  port: 3128\n",
	} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, enabled := range []bool{true, false} {
			if err := persistDesktopStartup(path, enabled); err != nil {
				t.Fatalf("input=%q: %v", input, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var config AppConfig
			if err := yaml.Unmarshal(data, &config); err != nil {
				t.Fatal(err)
			}
			if config.Desktop.StartAtLogin == nil || *config.Desktop.StartAtLogin != enabled || config.Proxy.Port != 3128 {
				t.Fatalf("reloaded = %+v", config)
			}
			for _, retained := range []string{"# keep", "# startup choice", "future_setting"} {
				if strings.Contains(input, retained) && !strings.Contains(string(data), retained) {
					t.Fatalf("lost %q from %q", retained, data)
				}
			}
		}
	}
}
