package configuration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestReadMCPConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		want MCPConfig
	}{
		{"legacy file", "proxy:\n  port: 3128\n", DefaultMCPConfig()},
		{"enabled with default port", "mcp:\n  enabled: true\n", MCPConfig{Enabled: true, Port: 8087}},
		{"custom port", "mcp:\n  enabled: true\n  port: 9091\n", MCPConfig{Enabled: true, Port: 9091}},
		{"disabled with custom port", "mcp:\n  enabled: false\n  port: 9091\n", MCPConfig{Port: 9091}},
		{"explicit zero is not defaulted", "mcp:\n  enabled: true\n  port: 0\n", MCPConfig{Enabled: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := readConfigurationFromPath(path)
			if err != nil {
				t.Fatal(err)
			}
			if got.MCP != tc.want {
				t.Fatalf("MCP = %+v, want %+v", got.MCP, tc.want)
			}
		})
	}
}

func TestDefaultConfigurationIncludesMCP(t *testing.T) {
	data, err := yaml.Marshal(DefaultAppConfig())
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		MCP *MCPConfig `yaml:"mcp"`
	}
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.MCP == nil || *got.MCP != DefaultMCPConfig() {
		t.Fatalf("generated MCP config = %+v", got.MCP)
	}
}
