package main

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// ErrInvalidConfig 表示配置 YAML 为空或仅包含空白字符。
	ErrInvalidConfig = errors.New("config yaml is empty")
	// ErrEngineRunning 表示引擎已在运行，不能重复启动。
	ErrEngineRunning = errors.New("engine is already running")
	// ErrEngineStarting 表示引擎正处于启动流程中。
	ErrEngineStarting = errors.New("engine is starting")
	// ErrEngineNotRunning 表示引擎尚未启动，无法执行 stop/reload。
	ErrEngineNotRunning = errors.New("engine is not running")
)

const defaultStatsRefreshInterval = 200 * time.Millisecond

// EngineStatus 对应 proto 的 StatusResponse，描述内核运行状态。
type EngineStatus struct {
	Running       bool
	Version       string
	UptimeSeconds int64
	MemoryBytes   int64
	Connections   int32
	Error         string
}

// EngineConnection 描述一条活跃连接快照。
type EngineConnection struct {
	ID            string
	Source        string
	Destination   string
	Proxy         string
	Rule          string
	UploadBytes   int64
	DownloadBytes int64
	StartTime     int64
}

// EngineTraffic 描述累计流量和实时速度快照。
type EngineTraffic struct {
	UploadTotal   int64
	DownloadTotal int64
	UploadSpeed   int64
	DownloadSpeed int64
}

// EngineManager 负责管理内核生命周期及运行时状态。
type EngineManager struct {
	mu sync.RWMutex

	runtime EngineRuntime

	version     string
	starting    bool
	running     bool
	startedAt   time.Time
	configYAML  string
	memoryBytes int64
	lastError   string

	connections map[string]EngineConnection
	traffic     EngineTraffic
	lastSample  trafficSample
	lastRefresh time.Time

	statsRefreshInterval time.Duration

	startupHook func()
}

type trafficSample struct {
	uploadTotal   int64
	downloadTotal int64
	timestamp     time.Time
}

// NewEngineManager 使用指定运行时创建内核管理器。
func NewEngineManager(runtime EngineRuntime) *EngineManager {
	if runtime == nil {
		panic("engine runtime is required")
	}

	version := strings.TrimSpace(runtime.Version())
	if version == "" {
		version = "mihomo-unknown"
	}

	return &EngineManager{
		runtime:              runtime,
		version:              version,
		connections:          make(map[string]EngineConnection),
		statsRefreshInterval: defaultStatsRefreshInterval,
	}
}

// SetLogHandler 为运行时日志设置回调（若运行时支持）。
func (manager *EngineManager) SetLogHandler(handler func(level, message string)) {
	if logAwareRuntime, ok := manager.runtime.(EngineLogAwareRuntime); ok {
		logAwareRuntime.SetLogHandler(handler)
	}
}

// Start 启动引擎并加载配置。
func (manager *EngineManager) Start(configYAML string) (EngineStatus, error) {
	if strings.TrimSpace(configYAML) == "" {
		return EngineStatus{}, ErrInvalidConfig
	}

	manager.mu.Lock()
	if manager.running {
		manager.mu.Unlock()
		return EngineStatus{}, ErrEngineRunning
	}
	if manager.starting {
		manager.mu.Unlock()
		return EngineStatus{}, ErrEngineStarting
	}

	manager.starting = true
	startupHook := manager.startupHook
	runtime := manager.runtime
	manager.mu.Unlock()

	if startupHook != nil {
		startupHook()
	}

	startErr := runtime.Start(configYAML)

	manager.mu.Lock()
	defer manager.mu.Unlock()

	manager.starting = false
	if startErr != nil {
		manager.running = false
		manager.lastError = startErr.Error()
		return EngineStatus{}, fmt.Errorf("runtime start failed: %w", startErr)
	}

	manager.running = true
	manager.startedAt = time.Now()
	manager.memoryBytes = 64 * 1024 * 1024
	manager.configYAML = configYAML
	manager.lastError = ""
	manager.lastSample = trafficSample{}
	manager.lastRefresh = time.Time{}
	manager.refreshRuntimeStatsLocked(true)

	return manager.statusLocked(), nil
}

// Stop 停止引擎并清理运行时状态。
func (manager *EngineManager) Stop() (EngineStatus, error) {
	manager.mu.Lock()
	if manager.starting {
		manager.mu.Unlock()
		return EngineStatus{}, ErrEngineStarting
	}
	if !manager.running {
		manager.mu.Unlock()
		return EngineStatus{}, ErrEngineNotRunning
	}
	runtime := manager.runtime
	manager.mu.Unlock()

	stopErr := runtime.Stop()
	if stopErr != nil {
		manager.mu.Lock()
		manager.lastError = stopErr.Error()
		manager.mu.Unlock()
		return EngineStatus{}, fmt.Errorf("runtime stop failed: %w", stopErr)
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()

	manager.running = false
	manager.startedAt = time.Time{}
	manager.memoryBytes = 0
	manager.connections = make(map[string]EngineConnection)
	manager.traffic = EngineTraffic{}
	manager.lastError = ""
	manager.lastSample = trafficSample{}
	manager.lastRefresh = time.Time{}

	return manager.statusLocked(), nil
}

// Reload 热重载配置（要求引擎已运行）。
func (manager *EngineManager) Reload(configYAML string) (EngineStatus, error) {
	if strings.TrimSpace(configYAML) == "" {
		return EngineStatus{}, ErrInvalidConfig
	}

	manager.mu.Lock()
	if manager.starting {
		manager.mu.Unlock()
		return EngineStatus{}, ErrEngineStarting
	}
	if !manager.running {
		manager.mu.Unlock()
		return EngineStatus{}, ErrEngineNotRunning
	}
	runtime := manager.runtime
	manager.mu.Unlock()

	reloadErr := runtime.Reload(configYAML)
	if reloadErr != nil {
		manager.mu.Lock()
		manager.lastError = reloadErr.Error()
		manager.mu.Unlock()
		return EngineStatus{}, fmt.Errorf("runtime reload failed: %w", reloadErr)
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()

	manager.configYAML = configYAML
	manager.lastError = ""
	manager.refreshRuntimeStatsLocked(true)
	return manager.statusLocked(), nil
}

// GetStatus 获取当前状态快照。
func (manager *EngineManager) GetStatus() EngineStatus {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.refreshRuntimeStatsLocked(false)
	return manager.statusLocked()
}

// GetConnections 获取活跃连接列表（按 ID 排序保证稳定输出）。
func (manager *EngineManager) GetConnections() []EngineConnection {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	manager.refreshRuntimeStatsLocked(false)

	connections := make([]EngineConnection, 0, len(manager.connections))
	for _, connection := range manager.connections {
		connections = append(connections, connection)
	}

	sort.Slice(connections, func(i, j int) bool {
		return compareConnectionID(connections[i].ID, connections[j].ID)
	})

	return connections
}

// GetTraffic 获取当前流量快照。
func (manager *EngineManager) GetTraffic() EngineTraffic {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.refreshRuntimeStatsLocked(false)
	return manager.traffic
}

// UpsertConnection 新增或更新连接记录。
func (manager *EngineManager) UpsertConnection(connection EngineConnection) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.connections[connection.ID] = connection
}

// SetTraffic 更新流量统计快照。
func (manager *EngineManager) SetTraffic(traffic EngineTraffic) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.traffic = traffic
}

// statusLocked 在持锁状态下生成状态对象。
func (manager *EngineManager) statusLocked() EngineStatus {
	var uptimeSeconds int64
	if manager.running {
		uptimeSeconds = int64(time.Since(manager.startedAt).Seconds())
	}

	errorMessage := manager.lastError
	if manager.starting {
		errorMessage = ErrEngineStarting.Error()
	}

	return EngineStatus{
		Running:       manager.running,
		Version:       manager.version,
		UptimeSeconds: uptimeSeconds,
		MemoryBytes:   manager.memoryBytes,
		Connections:   int32(len(manager.connections)),
		Error:         errorMessage,
	}
}

func (manager *EngineManager) refreshRuntimeStatsLocked(force bool) {
	if !manager.running {
		return
	}

	statsRuntime, ok := manager.runtime.(EngineStatsRuntime)
	if !ok {
		return
	}

	if !force &&
		!manager.lastRefresh.IsZero() &&
		time.Since(manager.lastRefresh) < manager.statsRefreshInterval {
		return
	}

	snapshot, err := statsRuntime.Snapshot()
	now := time.Now()
	manager.lastRefresh = now
	if err != nil {
		manager.lastError = err.Error()
		return
	}

	nextConnections := make(map[string]EngineConnection, len(snapshot.Connections))
	for _, connection := range snapshot.Connections {
		nextConnections[connection.ID] = connection
	}
	manager.connections = nextConnections

	manager.traffic.UploadTotal = snapshot.Traffic.UploadTotal
	manager.traffic.DownloadTotal = snapshot.Traffic.DownloadTotal

	if !manager.lastSample.timestamp.IsZero() {
		elapsedSeconds := now.Sub(manager.lastSample.timestamp).Seconds()
		if elapsedSeconds > 0 {
			uploadDelta := snapshot.Traffic.UploadTotal - manager.lastSample.uploadTotal
			downloadDelta := snapshot.Traffic.DownloadTotal - manager.lastSample.downloadTotal
			if uploadDelta < 0 {
				uploadDelta = 0
			}
			if downloadDelta < 0 {
				downloadDelta = 0
			}
			manager.traffic.UploadSpeed = int64(float64(uploadDelta) / elapsedSeconds)
			manager.traffic.DownloadSpeed = int64(float64(downloadDelta) / elapsedSeconds)
		}
	}

	manager.lastSample = trafficSample{
		uploadTotal:   snapshot.Traffic.UploadTotal,
		downloadTotal: snapshot.Traffic.DownloadTotal,
		timestamp:     now,
	}
	manager.memoryBytes = snapshot.MemoryBytes
	manager.lastError = ""
}

func compareConnectionID(leftID, rightID string) bool {
	leftPrefix, leftNumber, leftHasNumber := splitIDNumberSuffix(leftID)
	rightPrefix, rightNumber, rightHasNumber := splitIDNumberSuffix(rightID)

	if leftHasNumber && rightHasNumber && leftPrefix == rightPrefix {
		if leftNumber != rightNumber {
			return leftNumber < rightNumber
		}
	}

	return leftID < rightID
}

func splitIDNumberSuffix(id string) (string, int, bool) {
	index := len(id)
	for index > 0 {
		lastChar := id[index-1]
		if lastChar < '0' || lastChar > '9' {
			break
		}
		index--
	}

	if index == len(id) {
		return id, 0, false
	}

	number, err := strconv.Atoi(id[index:])
	if err != nil {
		return id, 0, false
	}

	return id[:index], number, true
}

func (manager *EngineManager) setStartupHook(hook func()) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.startupHook = hook
}

func (manager *EngineManager) setStatsRefreshInterval(interval time.Duration) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	if interval <= 0 {
		interval = defaultStatsRefreshInterval
	}
	manager.statsRefreshInterval = interval
}
