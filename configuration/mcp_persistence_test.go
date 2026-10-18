package configuration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPersistMCPConfig(t *testing.T) {
	want := MCPConfig{Enabled: true, Port: 9091, RequireToken: true, Token: "0a1b2c"}
	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{
			"updates keys in place and keeps comments",
			"proxy:\n  port: 3128\nmcp:\n  enabled: false # opt-in\n  port: 8087 # loopback only\n\nlogging:\n  level: info\n",
			"proxy:\n  port: 3128\nmcp:\n  enabled: true # opt-in\n  port: 9091 # loopback only\n  require_token: true\n  token: \"0a1b2c\"\n\nlogging:\n  level: info\n",
		},
		{
			"adds a missing section at the end",
			"proxy:\n  port: 3128",
			"proxy:\n  port: 3128\nmcp:\n  enabled: true\n  port: 9091\n  require_token: true\n  token: \"0a1b2c\"\n",
		},
		{
			"replaces an existing token",
			"mcp:\n    enabled: true\n    port: 9091\n    require_token: false\n    token: \"old\"\nstorage:\n    enable: true\n",
			"mcp:\n    enabled: true\n    port: 9091\n    require_token: true\n    token: \"0a1b2c\"\nstorage:\n    enable: true\n",
		},
		{
			"ignores keys of the same name in other sections",
			"proxy:\n  port: 3128\nmcp:\n  enabled: false\n",
			"proxy:\n  port: 3128\nmcp:\n  enabled: true\n  port: 9091\n  require_token: true\n  token: \"0a1b2c\"\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tc.input), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := persistMCPConfig(path, want); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tc.want {
				t.Fatalf("config =\n%s\nwant\n%s", data, tc.want)
			}
			reloaded, err := readConfigurationFromPath(path)
			if err != nil {
				t.Fatal(err)
			}
			if reloaded.MCP != want {
				t.Fatalf("reloaded MCP = %+v, want %+v", reloaded.MCP, want)
			}
		})
	}
}
