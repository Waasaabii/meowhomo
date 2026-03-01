package main

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestBuildMihomoConfigFromNodes(t *testing.T) {
	configYAML, err := BuildMihomoConfig(ConfigBuildInput{
		MixedPort: 7890,
		AllowLAN:  true,
		Mode:      "rule",
		LogLevel:  "info",
		Proxies: []ParsedNode{
			{
				Name:     "HK-SS",
				Type:     "ss",
				Server:   "1.2.3.4",
				Port:     8388,
				Cipher:   "aes-256-gcm",
				Password: "pass123",
			},
			{
				Name:     "JP-TROJAN",
				Type:     "trojan",
				Server:   "jp.example.com",
				Port:     443,
				Password: "token",
				SNI:      "jp.example.com",
				TLS:      true,
			},
		},
		ProxyGroups: []ProxyGroupSpec{
			{
				Name:    "GLOBAL",
				Type:    "select",
				Proxies: []string{"HK-SS", "JP-TROJAN"},
			},
		},
		Rules: []string{"MATCH,GLOBAL"},
	})
	if err != nil {
		t.Fatalf("build config failed: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(configYAML), &parsed); err != nil {
		t.Fatalf("unmarshal config yaml failed: %v", err)
	}

	if parsed["mixed-port"] != 7890 {
		t.Fatalf("unexpected mixed-port: %v", parsed["mixed-port"])
	}
	if parsed["mode"] != "rule" {
		t.Fatalf("unexpected mode: %v", parsed["mode"])
	}
	proxies, ok := parsed["proxies"].([]any)
	if !ok || len(proxies) != 2 {
		t.Fatalf("unexpected proxies payload: %v", parsed["proxies"])
	}
}

func TestBuildMihomoConfigRejectsInvalidPort(t *testing.T) {
	_, err := BuildMihomoConfig(ConfigBuildInput{MixedPort: 70000})
	if err == nil {
		t.Fatal("expected invalid mixed port error")
	}
}
