package main

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/Waasaabii/meowhomo/engine/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const testBufSize = 1024 * 1024

func setupGRPCTest(t *testing.T) (pb.MihomoEngineClient, *EngineManager, func()) {
	t.Helper()

	listener := bufconn.Listen(testBufSize)
	manager := NewEngineManager(newFakeRuntime("mihomo-test"))
	service := NewEngineGRPCServer(manager)

	grpcServer := grpc.NewServer()
	pb.RegisterMihomoEngineServer(grpcServer, service)

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			t.Logf("grpc server terminated: %v", err)
		}
	}()

	dialer := func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}
	connection, err := grpc.DialContext(
		context.Background(),
		"passthrough:///bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(dialer),
	)
	if err != nil {
		t.Fatalf("failed to dial bufconn: %v", err)
	}

	cleanup := func() {
		_ = connection.Close()
		grpcServer.Stop()
		_ = listener.Close()
	}

	return pb.NewMihomoEngineClient(connection), manager, cleanup
}

func TestGRPCStartReloadStatusLifecycle(t *testing.T) {
	client, _, cleanup := setupGRPCTest(t)
	defer cleanup()

	startResponse, err := client.Start(context.Background(), &pb.StartRequest{
		ConfigYaml: "mixed-port: 7890\nmode: rule\n",
	})
	if err != nil {
		t.Fatalf("start rpc failed: %v", err)
	}
	if !startResponse.GetRunning() {
		t.Fatalf("engine should be running after start")
	}

	_, err = client.Reload(context.Background(), &pb.ReloadRequest{ConfigYaml: "   "})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}

	reloadResponse, err := client.Reload(context.Background(), &pb.ReloadRequest{
		ConfigYaml: "mixed-port: 9000\nmode: global\n",
	})
	if err != nil {
		t.Fatalf("reload rpc failed: %v", err)
	}
	if !reloadResponse.GetRunning() {
		t.Fatalf("engine should stay running after reload")
	}

	statusResponse, err := client.GetStatus(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatalf("get status rpc failed: %v", err)
	}
	if !statusResponse.GetRunning() {
		t.Fatalf("status should report running")
	}

	stopResponse, err := client.Stop(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatalf("stop rpc failed: %v", err)
	}
	if stopResponse.GetRunning() {
		t.Fatalf("engine should stop")
	}
}

func TestGRPCStartStopPreconditions(t *testing.T) {
	client, _, cleanup := setupGRPCTest(t)
	defer cleanup()

	_, err := client.Stop(context.Background(), &pb.Empty{})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition on stop before start, got %v", err)
	}

	_, err = client.Start(context.Background(), &pb.StartRequest{
		ConfigYaml: "mixed-port: 7890\nmode: rule\n",
	})
	if err != nil {
		t.Fatalf("first start failed: %v", err)
	}

	_, err = client.Start(context.Background(), &pb.StartRequest{
		ConfigYaml: "mixed-port: 9000\nmode: global\n",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition on duplicate start, got %v", err)
	}
}

func TestGRPCStartWhileEngineStarting(t *testing.T) {
	client, manager, cleanup := setupGRPCTest(t)
	defer cleanup()

	enteredStartup := make(chan struct{})
	releaseStartup := make(chan struct{})
	manager.setStartupHook(func() {
		close(enteredStartup)
		<-releaseStartup
	})

	var waitGroup sync.WaitGroup
	waitGroup.Add(1)
	var firstStartErr error
	go func() {
		defer waitGroup.Done()
		_, firstStartErr = client.Start(context.Background(), &pb.StartRequest{
			ConfigYaml: "mixed-port: 7890\nmode: rule\n",
		})
	}()

	<-enteredStartup

	_, err := client.Start(context.Background(), &pb.StartRequest{
		ConfigYaml: "mixed-port: 9000\nmode: global\n",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition while engine is starting, got %v", err)
	}

	close(releaseStartup)
	waitGroup.Wait()
	if firstStartErr != nil {
		t.Fatalf("first start should succeed, got %v", firstStartErr)
	}
}

func TestGRPCConnectionsAndTraffic(t *testing.T) {
	client, manager, cleanup := setupGRPCTest(t)
	defer cleanup()

	manager.UpsertConnection(EngineConnection{
		ID:            "conn-1",
		Source:        "10.0.0.1:30000",
		Destination:   "1.1.1.1:443",
		Proxy:         "hk-01",
		Rule:          "MATCH",
		UploadBytes:   128,
		DownloadBytes: 256,
		StartTime:     1700000000,
	})
	manager.SetTraffic(EngineTraffic{
		UploadTotal:   1000,
		DownloadTotal: 2000,
		UploadSpeed:   30,
		DownloadSpeed: 40,
	})

	connectionsResponse, err := client.GetConnections(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatalf("get connections rpc failed: %v", err)
	}
	if len(connectionsResponse.GetConnections()) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(connectionsResponse.GetConnections()))
	}
	if connectionsResponse.GetConnections()[0].GetId() != "conn-1" {
		t.Fatalf("unexpected connection id")
	}

	trafficResponse, err := client.GetTraffic(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatalf("get traffic rpc failed: %v", err)
	}
	if trafficResponse.GetUploadTotal() != 1000 || trafficResponse.GetDownloadTotal() != 2000 {
		t.Fatalf("unexpected traffic totals: %+v", trafficResponse)
	}
}

func TestGRPCStreamLogsEmitsLifecycleEvents(t *testing.T) {
	client, _, cleanup := setupGRPCTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.StreamLogs(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("stream logs rpc failed: %v", err)
	}

	initialEntry, err := stream.Recv()
	if err != nil {
		t.Fatalf("stream should emit subscription event: %v", err)
	}
	if !strings.Contains(initialEntry.GetMessage(), "日志订阅已建立") {
		t.Fatalf("unexpected initial stream message: %s", initialEntry.GetMessage())
	}

	_, err = client.Start(context.Background(), &pb.StartRequest{
		ConfigYaml: "mixed-port: 7890\nmode: rule\n",
	})
	if err != nil {
		t.Fatalf("start rpc failed: %v", err)
	}

	foundStartLog := false
	for !foundStartLog {
		entry, recvErr := stream.Recv()
		if recvErr != nil {
			t.Fatalf("stream recv failed before matching log: %v", recvErr)
		}
		if strings.Contains(entry.GetMessage(), "启动成功") {
			foundStartLog = true
		}
	}
}

func TestClassifyLogSourceCoversAllRoles(t *testing.T) {
	testCases := []struct {
		name     string
		message  string
		expected string
	}{
		{name: "subscription", message: "subscription provider pull succeeded", expected: "订阅喵"},
		{name: "strategy", message: "proxy selector updated", expected: "策略喵"},
		{name: "port", message: "listener bind on mixed port", expected: "端口喵"},
		{name: "kernel", message: "core runtime started", expected: "内核喵"},
		{name: "notification", message: "notification: cert will expire soon", expected: "通知喵"},
		{name: "tls", message: "tls certificate handshake complete", expected: "TLS喵"},
		{name: "backup", message: "backup export finished", expected: "备份喵"},
		{name: "speedtest", message: "speedtest latency result is 45ms", expected: "测速喵"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			source := classifyLogSource(testCase.message)
			if source != testCase.expected {
				t.Fatalf("expected %s, got %s", testCase.expected, source)
			}
		})
	}
}
