package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ParsedNode 描述从分享链接中解析出来的标准化节点信息。
type ParsedNode struct {
	Name     string
	Type     string
	Server   string
	Port     int
	Password string
	Cipher   string
	UUID     string
	AlterID  int
	Network  string
	TLS      bool
	SNI      string
	Host     string
	Path     string
	RawURL   string
}

// ParseNodeBatch 将订阅文本（支持 base64 包装）解析为节点列表。
func ParseNodeBatch(payload string) ([]ParsedNode, error) {
	trimmedPayload := strings.TrimSpace(payload)
	if trimmedPayload == "" {
		return nil, nil
	}

	if decodedPayload, ok := decodeMaybeSubscriptionBase64(trimmedPayload); ok {
		trimmedPayload = decodedPayload
	}

	links := strings.FieldsFunc(trimmedPayload, func(char rune) bool {
		return char == '\n' || char == '\r' || char == ' ' || char == '\t'
	})

	nodes := make([]ParsedNode, 0, len(links))
	for index, link := range links {
		candidate := strings.TrimSpace(link)
		if candidate == "" || strings.HasPrefix(candidate, "#") {
			continue
		}

		node, err := ParseNodeLink(candidate)
		if err != nil {
			return nil, fmt.Errorf("parse link at index %d failed: %w", index, err)
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

// DetectNodeLinkPayload 判断输入是否看起来像节点链接集合或其 base64 形式。
func DetectNodeLinkPayload(payload string) bool {
	trimmedPayload := strings.TrimSpace(payload)
	if trimmedPayload == "" {
		return false
	}

	lowerPayload := strings.ToLower(trimmedPayload)
	if strings.Contains(lowerPayload, "ss://") ||
		strings.Contains(lowerPayload, "vmess://") ||
		strings.Contains(lowerPayload, "trojan://") {
		return true
	}

	_, ok := decodeMaybeSubscriptionBase64(trimmedPayload)
	return ok
}

// ParseNodeLink 解析单条 ss:// vmess:// trojan:// 分享链接。
func ParseNodeLink(rawLink string) (ParsedNode, error) {
	link := strings.TrimSpace(rawLink)
	if link == "" {
		return ParsedNode{}, fmt.Errorf("empty node link")
	}

	switch {
	case strings.HasPrefix(link, "ss://"):
		return parseShadowsocksLink(link)
	case strings.HasPrefix(link, "vmess://"):
		return parseVmessLink(link)
	case strings.HasPrefix(link, "trojan://"):
		return parseTrojanLink(link)
	default:
		return ParsedNode{}, fmt.Errorf("unsupported node link: %s", link)
	}
}

// ToMihomoProxy 将标准化节点转换成 mihomo 配置中的 proxy 条目。
func (node ParsedNode) ToMihomoProxy() map[string]any {
	proxy := map[string]any{
		"name":   node.Name,
		"type":   node.Type,
		"server": node.Server,
		"port":   node.Port,
	}

	switch node.Type {
	case "ss":
		proxy["cipher"] = node.Cipher
		proxy["password"] = node.Password
		proxy["udp"] = true
	case "vmess":
		proxy["uuid"] = node.UUID
		proxy["alterId"] = node.AlterID
		proxy["cipher"] = defaultString(node.Cipher, "auto")
		proxy["network"] = defaultString(node.Network, "tcp")
		proxy["tls"] = node.TLS
		proxy["udp"] = true
		if node.SNI != "" {
			proxy["servername"] = node.SNI
		}
		if node.Network == "ws" {
			wsOptions := map[string]any{
				"path": defaultString(node.Path, "/"),
			}
			if node.Host != "" {
				wsOptions["headers"] = map[string]string{"Host": node.Host}
			}
			proxy["ws-opts"] = wsOptions
		}
	case "trojan":
		proxy["password"] = node.Password
		proxy["network"] = defaultString(node.Network, "tcp")
		proxy["tls"] = true
		proxy["udp"] = true
		if node.SNI != "" {
			proxy["sni"] = node.SNI
		}
		if node.Network == "ws" {
			wsOptions := map[string]any{
				"path": defaultString(node.Path, "/"),
			}
			if node.Host != "" {
				wsOptions["headers"] = map[string]string{"Host": node.Host}
			}
			proxy["ws-opts"] = wsOptions
		}
	}

	return proxy
}

func parseShadowsocksLink(link string) (ParsedNode, error) {
	parsedURL, err := url.Parse(link)
	if err != nil {
		return ParsedNode{}, fmt.Errorf("parse shadowsocks url failed: %w", err)
	}

	name := decodeFragment(parsedURL.Fragment)
	if parsedURL.Host != "" {
		host, port, splitErr := splitHostPort(parsedURL.Host)
		if splitErr != nil {
			return ParsedNode{}, splitErr
		}

		method, password, parseErr := parseShadowsocksCredential(parsedURL.User.String())
		if parseErr != nil {
			return ParsedNode{}, parseErr
		}

		if name == "" {
			name = fmt.Sprintf("%s:%d", host, port)
		}

		return ParsedNode{
			Name:     name,
			Type:     "ss",
			Server:   host,
			Port:     port,
			Password: password,
			Cipher:   method,
			RawURL:   link,
		}, nil
	}

	raw := strings.TrimPrefix(link, "ss://")
	raw = strings.Split(raw, "#")[0]
	raw = strings.Split(raw, "?")[0]
	decoded, decodeErr := decodeBase64Auto(raw)
	if decodeErr != nil {
		return ParsedNode{}, fmt.Errorf("decode shadowsocks payload failed: %w", decodeErr)
	}

	segments := strings.Split(decoded, "@")
	if len(segments) != 2 {
		return ParsedNode{}, fmt.Errorf("invalid shadowsocks payload")
	}

	method, password, parseErr := parseShadowsocksCredential(segments[0])
	if parseErr != nil {
		return ParsedNode{}, parseErr
	}

	host, port, splitErr := splitHostPort(segments[1])
	if splitErr != nil {
		return ParsedNode{}, splitErr
	}

	if name == "" {
		name = fmt.Sprintf("%s:%d", host, port)
	}

	return ParsedNode{
		Name:     name,
		Type:     "ss",
		Server:   host,
		Port:     port,
		Password: password,
		Cipher:   method,
		RawURL:   link,
	}, nil
}

func parseVmessLink(link string) (ParsedNode, error) {
	type vmessPayload struct {
		Name    string `json:"ps"`
		Server  string `json:"add"`
		Port    string `json:"port"`
		UUID    string `json:"id"`
		AlterID string `json:"aid"`
		Net     string `json:"net"`
		TLS     string `json:"tls"`
		Host    string `json:"host"`
		Path    string `json:"path"`
		SNI     string `json:"sni"`
		Cipher  string `json:"scy"`
	}

	payload := strings.TrimPrefix(link, "vmess://")
	decodedPayload, err := decodeBase64Auto(payload)
	if err != nil {
		return ParsedNode{}, fmt.Errorf("decode vmess payload failed: %w", err)
	}

	var vmess vmessPayload
	if err := json.Unmarshal([]byte(decodedPayload), &vmess); err != nil {
		return ParsedNode{}, fmt.Errorf("unmarshal vmess payload failed: %w", err)
	}

	port, err := strconv.Atoi(strings.TrimSpace(vmess.Port))
	if err != nil || port < 1 || port > 65535 {
		return ParsedNode{}, fmt.Errorf("invalid vmess port: %s", vmess.Port)
	}

	alterID, _ := strconv.Atoi(strings.TrimSpace(vmess.AlterID))
	name := strings.TrimSpace(vmess.Name)
	if name == "" {
		name = fmt.Sprintf("%s:%d", vmess.Server, port)
	}

	tlsEnabled := strings.EqualFold(vmess.TLS, "tls") ||
		strings.EqualFold(vmess.TLS, "true") ||
		vmess.TLS == "1"

	return ParsedNode{
		Name:    name,
		Type:    "vmess",
		Server:  strings.TrimSpace(vmess.Server),
		Port:    port,
		UUID:    strings.TrimSpace(vmess.UUID),
		AlterID: alterID,
		Network: defaultString(strings.TrimSpace(vmess.Net), "tcp"),
		TLS:     tlsEnabled,
		SNI:     strings.TrimSpace(vmess.SNI),
		Host:    strings.TrimSpace(vmess.Host),
		Path:    strings.TrimSpace(vmess.Path),
		Cipher:  defaultString(strings.TrimSpace(vmess.Cipher), "auto"),
		RawURL:  link,
	}, nil
}

func parseTrojanLink(link string) (ParsedNode, error) {
	parsedURL, err := url.Parse(link)
	if err != nil {
		return ParsedNode{}, fmt.Errorf("parse trojan url failed: %w", err)
	}

	host, port, err := splitHostPort(parsedURL.Host)
	if err != nil {
		return ParsedNode{}, err
	}

	name := decodeFragment(parsedURL.Fragment)
	if name == "" {
		name = fmt.Sprintf("%s:%d", host, port)
	}

	network := parsedURL.Query().Get("type")
	if network == "" {
		network = "tcp"
	}
	sni := parsedURL.Query().Get("sni")
	if sni == "" {
		sni = parsedURL.Query().Get("peer")
	}

	return ParsedNode{
		Name:     name,
		Type:     "trojan",
		Server:   host,
		Port:     port,
		Password: parsedURL.User.Username(),
		Network:  network,
		TLS:      true,
		SNI:      sni,
		Host:     parsedURL.Query().Get("host"),
		Path:     parsedURL.Query().Get("path"),
		RawURL:   link,
	}, nil
}

func parseShadowsocksCredential(raw string) (string, string, error) {
	credential := strings.TrimSpace(raw)
	if credential == "" {
		return "", "", fmt.Errorf("empty shadowsocks credential")
	}

	decodedCredential, decodeErr := decodeBase64Auto(credential)
	if decodeErr == nil {
		credential = decodedCredential
	}

	parts := strings.SplitN(credential, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid shadowsocks credential")
	}

	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

func splitHostPort(hostPort string) (string, int, error) {
	host, portString, err := net.SplitHostPort(hostPort)
	if err != nil {
		return "", 0, fmt.Errorf("invalid host:port %s", hostPort)
	}

	port, err := strconv.Atoi(portString)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("invalid port %s", portString)
	}

	return host, port, nil
}

func decodeMaybeSubscriptionBase64(payload string) (string, bool) {
	decoded, err := decodeBase64Auto(payload)
	if err != nil {
		return "", false
	}

	trimmedDecoded := strings.TrimSpace(decoded)
	if strings.Contains(trimmedDecoded, "://") {
		return trimmedDecoded, true
	}

	return "", false
}

func decodeBase64Auto(payload string) (string, error) {
	candidate := strings.TrimSpace(payload)
	candidate = strings.TrimRight(candidate, "=")
	if candidate == "" {
		return "", fmt.Errorf("empty base64 payload")
	}

	paddedCandidate := candidate + strings.Repeat("=", (4-len(candidate)%4)%4)

	decoded, err := base64.StdEncoding.DecodeString(paddedCandidate)
	if err == nil {
		return string(decoded), nil
	}

	decoded, err = base64.RawStdEncoding.DecodeString(candidate)
	if err == nil {
		return string(decoded), nil
	}

	decoded, err = base64.URLEncoding.DecodeString(paddedCandidate)
	if err == nil {
		return string(decoded), nil
	}

	decoded, err = base64.RawURLEncoding.DecodeString(candidate)
	if err == nil {
		return string(decoded), nil
	}

	return "", err
}

func decodeFragment(fragment string) string {
	if fragment == "" {
		return ""
	}

	decodedFragment, err := url.QueryUnescape(fragment)
	if err != nil {
		return fragment
	}
	return decodedFragment
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
