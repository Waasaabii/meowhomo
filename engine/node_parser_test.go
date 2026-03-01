package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseShadowsocksLink(t *testing.T) {
	node, err := ParseNodeLink("ss://YWVzLTI1Ni1nY206cGFzczEyMw==@1.2.3.4:8388#HK-SS")
	if err != nil {
		t.Fatalf("parse ss link failed: %v", err)
	}

	if node.Type != "ss" {
		t.Fatalf("expected ss type, got %s", node.Type)
	}
	if node.Server != "1.2.3.4" || node.Port != 8388 {
		t.Fatalf("unexpected ss endpoint: %+v", node)
	}
	if node.Cipher != "aes-256-gcm" || node.Password != "pass123" {
		t.Fatalf("unexpected ss credential: %+v", node)
	}
}

func TestParseVmessLink(t *testing.T) {
	vmessPayload := map[string]string{
		"ps":   "JP-VMESS",
		"add":  "jp.example.com",
		"port": "443",
		"id":   "11111111-1111-1111-1111-111111111111",
		"aid":  "0",
		"net":  "ws",
		"tls":  "tls",
		"host": "cdn.example.com",
		"path": "/ws",
		"sni":  "jp.example.com",
		"scy":  "auto",
	}
	encodedPayload, err := json.Marshal(vmessPayload)
	if err != nil {
		t.Fatalf("marshal vmess payload failed: %v", err)
	}

	link := "vmess://" + base64.StdEncoding.EncodeToString(encodedPayload)
	node, err := ParseNodeLink(link)
	if err != nil {
		t.Fatalf("parse vmess link failed: %v", err)
	}

	if node.Type != "vmess" {
		t.Fatalf("expected vmess type, got %s", node.Type)
	}
	if !node.TLS || node.Network != "ws" {
		t.Fatalf("unexpected vmess transport: %+v", node)
	}
	if node.SNI != "jp.example.com" || node.Path != "/ws" {
		t.Fatalf("unexpected vmess tls/ws settings: %+v", node)
	}
}

func TestParseTrojanLink(t *testing.T) {
	link := "trojan://password@hk.example.com:443?type=ws&sni=hk.example.com&host=cdn.hk.com&path=/trojan#HK-TROJAN"
	node, err := ParseNodeLink(link)
	if err != nil {
		t.Fatalf("parse trojan link failed: %v", err)
	}

	if node.Type != "trojan" {
		t.Fatalf("expected trojan type, got %s", node.Type)
	}
	if node.Password != "password" || node.Server != "hk.example.com" || node.Port != 443 {
		t.Fatalf("unexpected trojan endpoint: %+v", node)
	}
	if node.Network != "ws" || node.SNI != "hk.example.com" {
		t.Fatalf("unexpected trojan transport: %+v", node)
	}
}

func TestParseNodeBatchWithBase64Subscription(t *testing.T) {
	ssLink := "ss://YWVzLTI1Ni1nY206cGFzczEyMw==@1.2.3.4:8388#HK-SS"
	trojanLink := "trojan://password@hk.example.com:443?type=tcp&sni=hk.example.com#HK-TROJAN"
	subscription := base64.StdEncoding.EncodeToString([]byte(strings.Join([]string{
		ssLink,
		trojanLink,
	}, "\n")))

	nodes, err := ParseNodeBatch(subscription)
	if err != nil {
		t.Fatalf("parse node batch failed: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	if nodes[0].Type != "ss" || nodes[1].Type != "trojan" {
		t.Fatalf("unexpected node types: %+v", nodes)
	}
}

func TestDetectNodeLinkPayload(t *testing.T) {
	if !DetectNodeLinkPayload("ss://YWVzLTI1Ni1nY206cGFzczEyMw==@1.2.3.4:8388#HK-SS") {
		t.Fatal("expected ss link to be detected")
	}

	base64Payload := base64.StdEncoding.EncodeToString([]byte("trojan://password@host:443#demo"))
	if !DetectNodeLinkPayload(base64Payload) {
		t.Fatal("expected base64 subscription payload to be detected")
	}

	if DetectNodeLinkPayload("mixed-port: 7890\nmode: rule\n") {
		t.Fatal("yaml config should not be detected as node payload")
	}
}
