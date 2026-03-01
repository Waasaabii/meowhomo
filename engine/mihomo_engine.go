package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/metacubex/mihomo/common/observable"
	mihomoconfig "github.com/metacubex/mihomo/config"
	C "github.com/metacubex/mihomo/constant"
	mihomohub "github.com/metacubex/mihomo/hub"
	mihomoexecutor "github.com/metacubex/mihomo/hub/executor"
	mihomolog "github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/tunnel/statistic"
)

// EngineRuntime 定义引擎运行时接口。
type EngineRuntime interface {
	Start(configYAML string) error
	Reload(configYAML string) error
	Stop() error
	Version() string
}

// EngineLogAwareRuntime 表示运行时支持实时日志回调。
type EngineLogAwareRuntime interface {
	SetLogHandler(handler func(level, message string))
}

// EngineStatsRuntime 表示运行时可提供连接和流量快照。
type EngineStatsRuntime interface {
	Snapshot() (RuntimeSnapshot, error)
}

// RuntimeSnapshot 描述运行时统计快照。
type RuntimeSnapshot struct {
	Connections []EngineConnection
	Traffic     EngineTraffic
	MemoryBytes int64
}

// MihomoRuntimeOptions 描述真实 mihomo 运行时参数。
type MihomoRuntimeOptions struct {
	HomeDir                string
	ConfigPath             string
	ExternalController     string
	ExternalControllerUnix string
	ExternalControllerPipe string
	ExternalUI             string
	Secret                 string
}

// MihomoRuntime 直接桥接 github.com/metacubex/mihomo 的库级 API。
type MihomoRuntime struct {
	mu         sync.Mutex
	options    MihomoRuntimeOptions
	started    bool
	logHandler func(level, message string)
	logSub     observable.Subscription[mihomolog.Event]
}

// NewMihomoRuntime 创建真实 mihomo 运行时。
func NewMihomoRuntime(options MihomoRuntimeOptions) *MihomoRuntime {
	return &MihomoRuntime{
		options: options,
	}
}

func (runtime *MihomoRuntime) Version() string {
	return C.Version
}

// SetLogHandler 设置运行时日志回调。
func (runtime *MihomoRuntime) SetLogHandler(handler func(level, message string)) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	runtime.logHandler = handler
}

func (runtime *MihomoRuntime) Start(configYAML string) error {
	if strings.TrimSpace(configYAML) == "" {
		return ErrInvalidConfig
	}

	runtime.mu.Lock()
	if runtime.started {
		runtime.mu.Unlock()
		return ErrEngineRunning
	}

	logSub := mihomolog.Subscribe()
	runtime.logSub = logSub
	runtime.started = true
	runtime.mu.Unlock()

	go runtime.forwardLogs(logSub)

	if err := runtime.applyConfig(configYAML); err != nil {
		runtime.mu.Lock()
		if runtime.logSub != nil {
			mihomolog.UnSubscribe(runtime.logSub)
			runtime.logSub = nil
		}
		runtime.started = false
		runtime.mu.Unlock()
		return err
	}

	return nil
}

func (runtime *MihomoRuntime) Reload(configYAML string) error {
	if strings.TrimSpace(configYAML) == "" {
		return ErrInvalidConfig
	}

	runtime.mu.Lock()
	isStarted := runtime.started
	runtime.mu.Unlock()
	if !isStarted {
		return ErrEngineNotRunning
	}

	return runtime.applyConfig(configYAML)
}

func (runtime *MihomoRuntime) Stop() error {
	runtime.mu.Lock()
	if !runtime.started {
		runtime.mu.Unlock()
		return ErrEngineNotRunning
	}

	logSub := runtime.logSub
	runtime.logSub = nil
	runtime.started = false
	runtime.mu.Unlock()

	mihomoexecutor.Shutdown()
	if logSub != nil {
		mihomolog.UnSubscribe(logSub)
	}
	return nil
}

func (runtime *MihomoRuntime) Snapshot() (RuntimeSnapshot, error) {
	snapshot := statistic.DefaultManager.Snapshot()
	connections := make([]EngineConnection, 0, len(snapshot.Connections))

	for _, trackerInfo := range snapshot.Connections {
		if trackerInfo == nil {
			continue
		}

		source := ""
		destination := ""
		if trackerInfo.Metadata != nil {
			source = trackerInfo.Metadata.SourceAddress()
			destination = trackerInfo.Metadata.RemoteAddress()
		}

		proxy := trackerInfo.Chain.Last()
		if proxy == "" {
			proxy = trackerInfo.Chain.String()
		}

		rule := strings.TrimSpace(trackerInfo.Rule)
		if trackerInfo.RulePayload != "" {
			rule = strings.TrimSpace(fmt.Sprintf("%s,%s", trackerInfo.Rule, trackerInfo.RulePayload))
		}

		connections = append(connections, EngineConnection{
			ID:            trackerInfo.UUID.String(),
			Source:        source,
			Destination:   destination,
			Proxy:         proxy,
			Rule:          rule,
			UploadBytes:   trackerInfo.UploadTotal.Load(),
			DownloadBytes: trackerInfo.DownloadTotal.Load(),
			StartTime:     trackerInfo.Start.Unix(),
		})
	}

	return RuntimeSnapshot{
		Connections: connections,
		Traffic: EngineTraffic{
			UploadTotal:   snapshot.UploadTotal,
			DownloadTotal: snapshot.DownloadTotal,
		},
		MemoryBytes: int64(snapshot.Memory),
	}, nil
}

func (runtime *MihomoRuntime) applyConfig(configYAML string) error {
	if err := validateConfigYAML(configYAML); err != nil {
		return err
	}

	if err := runtime.prepareHomeDir(); err != nil {
		return err
	}

	options := runtime.buildHubOptions()
	if err := mihomohub.Parse([]byte(configYAML), options...); err != nil {
		return fmt.Errorf("mihomo parse failed: %w", err)
	}

	return nil
}

func validateConfigYAML(configYAML string) error {
	var parsed any
	if err := yaml.Unmarshal([]byte(configYAML), &parsed); err != nil {
		return fmt.Errorf("config yaml syntax invalid: %w", err)
	}

	if _, isMap := parsed.(map[string]any); !isMap {
		return fmt.Errorf("config yaml root must be a mapping")
	}

	return nil
}

func (runtime *MihomoRuntime) prepareHomeDir() error {
	homeDir := strings.TrimSpace(runtime.options.HomeDir)
	if homeDir == "" {
		return nil
	}

	if !filepath.IsAbs(homeDir) {
		absoluteHomeDir, err := filepath.Abs(homeDir)
		if err != nil {
			return fmt.Errorf("resolve home dir failed: %w", err)
		}
		homeDir = absoluteHomeDir
	}

	C.SetHomeDir(homeDir)
	if err := mihomoconfig.Init(homeDir); err != nil {
		return fmt.Errorf("init mihomo home dir failed: %w", err)
	}

	configPath := strings.TrimSpace(runtime.options.ConfigPath)
	if configPath != "" {
		if !filepath.IsAbs(configPath) {
			absoluteConfigPath, err := filepath.Abs(configPath)
			if err != nil {
				return fmt.Errorf("resolve config path failed: %w", err)
			}
			configPath = absoluteConfigPath
		}
		C.SetConfig(configPath)
	}

	return nil
}

func (runtime *MihomoRuntime) buildHubOptions() []mihomohub.Option {
	options := make([]mihomohub.Option, 0, 5)
	if runtime.options.ExternalUI != "" {
		options = append(options, mihomohub.WithExternalUI(runtime.options.ExternalUI))
	}
	if runtime.options.ExternalController != "" {
		options = append(options, mihomohub.WithExternalController(runtime.options.ExternalController))
	}
	if runtime.options.ExternalControllerUnix != "" {
		options = append(options, mihomohub.WithExternalControllerUnix(runtime.options.ExternalControllerUnix))
	}
	if runtime.options.ExternalControllerPipe != "" {
		options = append(options, mihomohub.WithExternalControllerPipe(runtime.options.ExternalControllerPipe))
	}
	if runtime.options.Secret != "" {
		options = append(options, mihomohub.WithSecret(runtime.options.Secret))
	}
	return options
}

func (runtime *MihomoRuntime) forwardLogs(logSub observable.Subscription[mihomolog.Event]) {
	for event := range logSub {
		runtime.mu.Lock()
		logHandler := runtime.logHandler
		runtime.mu.Unlock()

		if logHandler == nil {
			continue
		}

		logHandler(
			strings.ToLower(event.LogLevel.String()),
			strings.TrimSpace(event.Payload),
		)
	}
}
