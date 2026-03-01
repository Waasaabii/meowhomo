package main

import (
	"strings"
	"testing"
)

func TestBuildTrojanInboundDomainMode(t *testing.T) {
	inbound, err := BuildTrojanInbound(TrojanInboundRequest{
		Name:     "Local-Trojan",
		Host:     "proxy.example.com",
		Port:     443,
		Password: "secret",
		Mode:     LocalNodeModeDomainTLS,
	})
	if err != nil {
		t.Fatalf("build trojan inbound failed: %v", err)
	}

	if inbound.SNI != "proxy.example.com" {
		t.Fatalf("expected SNI fallback to host, got %s", inbound.SNI)
	}
	link := inbound.ShareLink()
	if !strings.HasPrefix(link, "trojan://") || !strings.Contains(link, "sni=proxy.example.com") {
		t.Fatalf("unexpected share link: %s", link)
	}
}

func TestBuildTrojanInboundRejectsInvalidPort(t *testing.T) {
	_, err := BuildTrojanInbound(TrojanInboundRequest{
		Name:     "broken",
		Host:     "127.0.0.1",
		Port:     70000,
		Password: "secret",
	})
	if err == nil {
		t.Fatal("expected invalid port error")
	}
}

func TestTrojanInboundToParsedNode(t *testing.T) {
	inbound, err := BuildTrojanInbound(TrojanInboundRequest{
		Name:     "Local-IP",
		Host:     "127.0.0.1",
		Port:     9443,
		Password: "token",
		Mode:     LocalNodeModeIPDirect,
		SNI:      "node.local",
	})
	if err != nil {
		t.Fatalf("build trojan inbound failed: %v", err)
	}

	node := inbound.ToParsedNode()
	if node.Type != "trojan" || node.Server != "127.0.0.1" || node.Port != 9443 {
		t.Fatalf("unexpected converted node: %+v", node)
	}
}
