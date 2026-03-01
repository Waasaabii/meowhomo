package main

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProxyGroupSpec 描述 mihomo 的 proxy-groups 条目。
type ProxyGroupSpec struct {
	Name     string
	Type     string
	Proxies  []string
	URL      string
	Interval int
}

// ConfigBuildInput 描述生成 mihomo YAML 所需的输入。
type ConfigBuildInput struct {
	MixedPort          int
	AllowLAN           bool
	Mode               string
	LogLevel           string
	ExternalController string
	Secret             string
	Proxies            []ParsedNode
	ProxyGroups        []ProxyGroupSpec
	Rules              []string
}

// BuildMihomoConfig 将标准化节点组装成 mihomo 可直接加载的 YAML。
func BuildMihomoConfig(input ConfigBuildInput) (string, error) {
	if input.MixedPort == 0 {
		input.MixedPort = 7890
	}
	if input.MixedPort < 1 || input.MixedPort > 65535 {
		return "", fmt.Errorf("invalid mixed port: %d", input.MixedPort)
	}
	if input.Mode == "" {
		input.Mode = "rule"
	}
	if input.LogLevel == "" {
		input.LogLevel = "info"
	}
	if len(input.Rules) == 0 {
		input.Rules = []string{"MATCH,DIRECT"}
	}

	root := map[string]any{
		"mixed-port": input.MixedPort,
		"allow-lan":  input.AllowLAN,
		"mode":       input.Mode,
		"log-level":  input.LogLevel,
	}

	if input.ExternalController != "" {
		root["external-controller"] = input.ExternalController
	}
	if input.Secret != "" {
		root["secret"] = input.Secret
	}

	proxies := make([]map[string]any, 0, len(input.Proxies))
	proxyNames := make([]string, 0, len(input.Proxies))
	for _, proxyNode := range input.Proxies {
		proxies = append(proxies, proxyNode.ToMihomoProxy())
		proxyNames = append(proxyNames, proxyNode.Name)
	}
	root["proxies"] = proxies

	proxyGroups := make([]map[string]any, 0, len(input.ProxyGroups)+1)
	for _, groupSpec := range input.ProxyGroups {
		if groupSpec.Name == "" {
			return "", fmt.Errorf("proxy group name cannot be empty")
		}

		group := map[string]any{
			"name":    groupSpec.Name,
			"type":    defaultString(groupSpec.Type, "select"),
			"proxies": groupSpec.Proxies,
		}
		if len(groupSpec.Proxies) == 0 {
			group["proxies"] = proxyNames
		}
		if groupSpec.URL != "" {
			group["url"] = groupSpec.URL
		}
		if groupSpec.Interval > 0 {
			group["interval"] = groupSpec.Interval
		}
		proxyGroups = append(proxyGroups, group)
	}

	if len(proxyGroups) == 0 {
		proxyGroups = append(proxyGroups, map[string]any{
			"name":    "AUTO",
			"type":    "select",
			"proxies": proxyNames,
		})
	}
	root["proxy-groups"] = proxyGroups
	root["rules"] = input.Rules

	encodedYAML, err := yaml.Marshal(root)
	if err != nil {
		return "", fmt.Errorf("marshal config yaml failed: %w", err)
	}

	return strings.TrimSpace(string(encodedYAML)) + "\n", nil
}
