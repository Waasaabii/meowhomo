package main

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// LocalNodeMode 标识本机节点创建模式。
type LocalNodeMode string

const (
	// LocalNodeModeIPDirect 表示 IP 直连模式。
	LocalNodeModeIPDirect LocalNodeMode = "ip-direct"
	// LocalNodeModeDomainTLS 表示域名 TLS 模式。
	LocalNodeModeDomainTLS LocalNodeMode = "domain-tls"
)

// TrojanInboundRequest 描述创建本机 Trojan 节点所需参数。
type TrojanInboundRequest struct {
	Name               string
	Host               string
	Port               int
	Password           string
	Mode               LocalNodeMode
	SNI                string
	SkipCertVerify     bool
	ClientFingerprint  string
	SubscriptionPrefix string
}

// TrojanInbound 描述创建好的本机 Trojan 节点。
type TrojanInbound struct {
	Name              string
	Host              string
	Port              int
	Password          string
	Mode              LocalNodeMode
	SNI               string
	SkipCertVerify    bool
	ClientFingerprint string
}

// BuildTrojanInbound 校验并创建本机 Trojan 节点定义。
func BuildTrojanInbound(request TrojanInboundRequest) (TrojanInbound, error) {
	name := strings.TrimSpace(request.Name)
	host := strings.TrimSpace(request.Host)
	password := strings.TrimSpace(request.Password)

	if name == "" {
		return TrojanInbound{}, fmt.Errorf("trojan inbound name is required")
	}
	if host == "" {
		return TrojanInbound{}, fmt.Errorf("trojan inbound host is required")
	}
	if request.Port < 1 || request.Port > 65535 {
		return TrojanInbound{}, fmt.Errorf("trojan inbound port is invalid: %d", request.Port)
	}
	if password == "" {
		return TrojanInbound{}, fmt.Errorf("trojan inbound password is required")
	}

	mode := request.Mode
	if mode == "" {
		mode = LocalNodeModeDomainTLS
	}
	if mode != LocalNodeModeDomainTLS && mode != LocalNodeModeIPDirect {
		return TrojanInbound{}, fmt.Errorf("unsupported local node mode: %s", mode)
	}

	sni := strings.TrimSpace(request.SNI)
	if mode == LocalNodeModeDomainTLS && sni == "" {
		sni = host
	}

	return TrojanInbound{
		Name:              name,
		Host:              host,
		Port:              request.Port,
		Password:          password,
		Mode:              mode,
		SNI:               sni,
		SkipCertVerify:    request.SkipCertVerify,
		ClientFingerprint: strings.TrimSpace(request.ClientFingerprint),
	}, nil
}

// ShareLink 返回 Trojan 标准分享链接。
func (inbound TrojanInbound) ShareLink() string {
	values := url.Values{}
	values.Set("security", "tls")
	values.Set("type", "tcp")
	if inbound.SNI != "" {
		values.Set("sni", inbound.SNI)
	}
	if inbound.SkipCertVerify {
		values.Set("allowInsecure", "1")
	}

	hostPort := net.JoinHostPort(inbound.Host, strconv.Itoa(inbound.Port))
	return fmt.Sprintf(
		"trojan://%s@%s?%s#%s",
		url.QueryEscape(inbound.Password),
		hostPort,
		values.Encode(),
		url.QueryEscape(inbound.Name),
	)
}

// ToParsedNode 转换为标准化节点，用于写入 proxy 列表。
func (inbound TrojanInbound) ToParsedNode() ParsedNode {
	return ParsedNode{
		Name:     inbound.Name,
		Type:     "trojan",
		Server:   inbound.Host,
		Port:     inbound.Port,
		Password: inbound.Password,
		Network:  "tcp",
		TLS:      true,
		SNI:      inbound.SNI,
	}
}
