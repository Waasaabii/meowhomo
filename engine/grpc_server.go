package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	pb "github.com/Waasaabii/meowhomo/engine/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	subscriptionLogKeywords = []string{
		"subscription", "subscribe", "provider", "pull", "fetch",
		"订阅", "拉取",
	}
	strategyLogKeywords = []string{
		"proxy", "selector", "fallback", "strategy", "rule", "policy",
		"策略", "规则",
	}
	portLogKeywords = []string{
		"listener", "port", "bind", "inbound", "socks", "http", "tproxy",
		"端口", "监听",
	}
	notificationLogKeywords = []string{
		"notify", "notification", "alert", "warn", "notice", "expire", "expired",
		"通知", "提醒", "过期",
	}
	tlsLogKeywords = []string{
		"tls", "ssl", "cert", "certificate", "handshake", "acme", "encrypt",
		"证书", "加密",
	}
	backupLogKeywords = []string{
		"backup", "export", "import", "restore", "archive",
		"备份", "导出", "导入", "恢复",
	}
	speedtestLogKeywords = []string{
		"speedtest", "latency", "delay", "benchmark", "ping",
		"测速", "延迟",
	}
)

type EngineGRPCServer struct {
	pb.UnimplementedMihomoEngineServer
	manager *EngineManager
	logHub  *LogHub
}

// NewEngineGRPCServer 创建 gRPC 服务实例，负责桥接 RPC 与 EngineManager。
func NewEngineGRPCServer(manager *EngineManager) *EngineGRPCServer {
	server := &EngineGRPCServer{
		manager: manager,
		logHub:  NewLogHub(),
	}

	manager.SetLogHandler(func(level, message string) {
		server.logHub.Publish(classifyLogSource(message), normalizeLogLevel(level), message)
	})

	return server
}

// Start 启动引擎并返回最新状态。
func (server *EngineGRPCServer) Start(
	_ context.Context,
	request *pb.StartRequest,
) (*pb.StatusResponse, error) {
	engineStatus, err := server.manager.Start(request.GetConfigYaml())
	if err != nil {
		return nil, mapEngineError(err)
	}

	server.logHub.Publish("内核喵", "info", "mihomo 引擎启动成功")
	return mapStatusResponse(engineStatus), nil
}

// Stop 停止引擎并返回最新状态。
func (server *EngineGRPCServer) Stop(
	_ context.Context,
	_ *pb.Empty,
) (*pb.StatusResponse, error) {
	engineStatus, err := server.manager.Stop()
	if err != nil {
		return nil, mapEngineError(err)
	}

	server.logHub.Publish("内核喵", "info", "mihomo 引擎已停止")
	return mapStatusResponse(engineStatus), nil
}

// Reload 热重载引擎配置。
func (server *EngineGRPCServer) Reload(
	_ context.Context,
	request *pb.ReloadRequest,
) (*pb.StatusResponse, error) {
	engineStatus, err := server.manager.Reload(request.GetConfigYaml())
	if err != nil {
		return nil, mapEngineError(err)
	}

	server.logHub.Publish("内核喵", "info", "mihomo 配置热重载完成")
	return mapStatusResponse(engineStatus), nil
}

// GetStatus 返回当前状态快照。
func (server *EngineGRPCServer) GetStatus(
	_ context.Context,
	_ *pb.Empty,
) (*pb.StatusResponse, error) {
	engineStatus := server.manager.GetStatus()
	return mapStatusResponse(engineStatus), nil
}

// GetConnections 返回当前活跃连接列表。
func (server *EngineGRPCServer) GetConnections(
	_ context.Context,
	_ *pb.Empty,
) (*pb.ConnectionsResponse, error) {
	engineConnections := server.manager.GetConnections()
	connections := make([]*pb.Connection, 0, len(engineConnections))
	for _, connection := range engineConnections {
		connections = append(connections, &pb.Connection{
			Id:            connection.ID,
			Source:        connection.Source,
			Destination:   connection.Destination,
			Proxy:         connection.Proxy,
			Rule:          connection.Rule,
			UploadBytes:   connection.UploadBytes,
			DownloadBytes: connection.DownloadBytes,
			StartTime:     connection.StartTime,
		})
	}

	return &pb.ConnectionsResponse{Connections: connections}, nil
}

// GetTraffic 返回当前流量统计。
func (server *EngineGRPCServer) GetTraffic(
	_ context.Context,
	_ *pb.Empty,
) (*pb.TrafficResponse, error) {
	engineTraffic := server.manager.GetTraffic()
	return &pb.TrafficResponse{
		UploadTotal:   engineTraffic.UploadTotal,
		DownloadTotal: engineTraffic.DownloadTotal,
		UploadSpeed:   engineTraffic.UploadSpeed,
		DownloadSpeed: engineTraffic.DownloadSpeed,
	}, nil
}

// StreamLogs 建立日志流，将 LogHub 中的事件持续推送给客户端。
func (server *EngineGRPCServer) StreamLogs(
	_ *pb.Empty,
	stream grpc.ServerStreamingServer[pb.LogEntry],
) error {
	subscriptionID, channel := server.logHub.Subscribe()
	defer server.logHub.Unsubscribe(subscriptionID)

	server.logHub.Publish("通知喵", "info", fmt.Sprintf("日志订阅已建立 #%d", subscriptionID))

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case logEntry, ok := <-channel:
			if !ok {
				return nil
			}
			if err := stream.Send(logEntry); err != nil {
				return status.Errorf(codes.Unavailable, "stream send failed: %v", err)
			}
		}
	}
}

// mapStatusResponse 将内核状态映射为 gRPC 响应对象。
func mapStatusResponse(engineStatus EngineStatus) *pb.StatusResponse {
	return &pb.StatusResponse{
		Running:       engineStatus.Running,
		Version:       engineStatus.Version,
		UptimeSeconds: engineStatus.UptimeSeconds,
		MemoryBytes:   engineStatus.MemoryBytes,
		Connections:   engineStatus.Connections,
		Error:         engineStatus.Error,
	}
}

// mapEngineError 将内核错误映射为 gRPC status code。
func mapEngineError(err error) error {
	switch {
	case errors.Is(err, ErrInvalidConfig):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ErrEngineStarting):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, ErrEngineRunning):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, ErrEngineNotRunning):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func classifyLogSource(message string) string {
	lowerMessage := strings.ToLower(message)
	switch {
	case containsAny(lowerMessage, subscriptionLogKeywords):
		return "订阅喵"
	case containsAny(lowerMessage, notificationLogKeywords):
		return "通知喵"
	case containsAny(lowerMessage, backupLogKeywords):
		return "备份喵"
	case containsAny(lowerMessage, speedtestLogKeywords):
		return "测速喵"
	case containsAny(lowerMessage, strategyLogKeywords):
		return "策略喵"
	case containsAny(lowerMessage, portLogKeywords):
		return "端口喵"
	case containsAny(lowerMessage, tlsLogKeywords):
		return "TLS喵"
	default:
		return "内核喵"
	}
}

func containsAny(message string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}

func normalizeLogLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return "info"
	case "warning":
		return "warn"
	case "error":
		return "error"
	default:
		return "info"
	}
}
