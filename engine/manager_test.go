package main

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestStartRequiresConfig(t *testing.T) {
	manager := NewEngineManager(newFakeRuntime("mihomo-test"))

	_, err := manager.Start("   ")
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestStartRejectsWhenAlreadyRunning(t *testing.T) {
	manager := NewEngineManager(newFakeRuntime("mihomo-test"))
	if _, err := manager.Start("mixed-port: 7890"); err != nil {
		t.Fatalf("first start failed: %v", err)
	}

	_, err := manager.Start("mixed-port: 9000")
	if !errors.Is(err, ErrEngineRunning) {
		t.Fatalf("expected ErrEngineRunning, got %v", err)
	}
}

func TestReloadRequiresRunningEngine(t *testing.T) {
	manager := NewEngineManager(newFakeRuntime("mihomo-test"))

	_, err := manager.Reload("mixed-port: 7890")
	if !errors.Is(err, ErrEngineNotRunning) {
		t.Fatalf("expected ErrEngineNotRunning, got %v", err)
	}
}

func TestStopRequiresRunningEngine(t *testing.T) {
	manager := NewEngineManager(newFakeRuntime("mihomo-test"))

	_, err := manager.Stop()
	if !errors.Is(err, ErrEngineNotRunning) {
		t.Fatalf("expected ErrEngineNotRunning, got %v", err)
	}
}

func TestStartRejectsWhenEngineStarting(t *testing.T) {
	manager := NewEngineManager(newFakeRuntime("mihomo-test"))

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
		_, firstStartErr = manager.Start("mixed-port: 7890")
	}()

	<-enteredStartup

	_, err := manager.Start("mixed-port: 9000")
	if !errors.Is(err, ErrEngineStarting) {
		t.Fatalf("expected ErrEngineStarting for duplicate start, got %v", err)
	}

	_, err = manager.Reload("mixed-port: 9000")
	if !errors.Is(err, ErrEngineStarting) {
		t.Fatalf("expected ErrEngineStarting for reload during startup, got %v", err)
	}

	_, err = manager.Stop()
	if !errors.Is(err, ErrEngineStarting) {
		t.Fatalf("expected ErrEngineStarting for stop during startup, got %v", err)
	}

	close(releaseStartup)
	waitGroup.Wait()
	if firstStartErr != nil {
		t.Fatalf("first start should succeed, got %v", firstStartErr)
	}
}

func TestStartReloadStopLifecycle(t *testing.T) {
	manager := NewEngineManager(newFakeRuntime("mihomo-test"))

	startStatus, err := manager.Start("mixed-port: 7890")
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if !startStatus.Running {
		t.Fatalf("engine should be running after start")
	}
	if startStatus.MemoryBytes == 0 {
		t.Fatalf("expected memory usage after start")
	}

	reloadStatus, err := manager.Reload("mixed-port: 9000")
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if !reloadStatus.Running {
		t.Fatalf("engine should stay running after reload")
	}

	stopStatus, err := manager.Stop()
	if err != nil {
		t.Fatalf("stop failed: %v", err)
	}
	if stopStatus.Running {
		t.Fatalf("engine should be stopped")
	}
	if stopStatus.MemoryBytes != 0 {
		t.Fatalf("memory should be reset on stop")
	}
}

func TestConnectionsAndTrafficSnapshot(t *testing.T) {
	manager := NewEngineManager(newFakeRuntime("mihomo-test"))
	if _, err := manager.Start("mixed-port: 7890"); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	manager.UpsertConnection(EngineConnection{
		ID:            "conn-2",
		Source:        "10.0.0.1:12345",
		Destination:   "8.8.8.8:443",
		Proxy:         "hk-01",
		Rule:          "MATCH",
		UploadBytes:   100,
		DownloadBytes: 200,
		StartTime:     1700000000,
	})
	manager.UpsertConnection(EngineConnection{
		ID:            "conn-10",
		Source:        "10.0.0.10:12345",
		Destination:   "8.8.4.4:443",
		Proxy:         "sg-01",
		Rule:          "MATCH",
		UploadBytes:   300,
		DownloadBytes: 400,
		StartTime:     1700000200,
	})
	manager.UpsertConnection(EngineConnection{
		ID:            "conn-1",
		Source:        "10.0.0.2:12345",
		Destination:   "1.1.1.1:443",
		Proxy:         "jp-01",
		Rule:          "GEOIP,CN",
		UploadBytes:   10,
		DownloadBytes: 20,
		StartTime:     1700000100,
	})
	manager.SetTraffic(EngineTraffic{
		UploadTotal:   11,
		DownloadTotal: 22,
		UploadSpeed:   33,
		DownloadSpeed: 44,
	})

	connections := manager.GetConnections()
	if len(connections) != 3 {
		t.Fatalf("expected 3 connections, got %d", len(connections))
	}
	if connections[0].ID != "conn-1" || connections[1].ID != "conn-2" || connections[2].ID != "conn-10" {
		t.Fatalf("connections should be sorted in natural id order, got: %+v", connections)
	}

	traffic := manager.GetTraffic()
	if traffic.UploadTotal != 11 || traffic.DownloadTotal != 22 {
		t.Fatalf("unexpected traffic totals: %+v", traffic)
	}

	status := manager.GetStatus()
	if status.Connections != 3 {
		t.Fatalf("expected connection count 3, got %d", status.Connections)
	}
}

func TestRuntimeSnapshotRefreshesTrafficAndConnections(t *testing.T) {
	runtime := newFakeStatsRuntime("fake-v1")
	manager := NewEngineManager(runtime)
	manager.setStatsRefreshInterval(10 * time.Millisecond)

	runtime.setSnapshot(RuntimeSnapshot{
		Connections: []EngineConnection{
			{
				ID:            "conn-2",
				Source:        "10.0.0.2:1000",
				Destination:   "1.1.1.1:443",
				Proxy:         "hk-01",
				Rule:          "MATCH",
				UploadBytes:   10,
				DownloadBytes: 20,
				StartTime:     1700000000,
			},
		},
		Traffic: EngineTraffic{
			UploadTotal:   100,
			DownloadTotal: 200,
		},
		MemoryBytes: 12345,
	})

	if _, err := manager.Start("mixed-port: 7890"); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	_ = manager.GetTraffic()

	time.Sleep(30 * time.Millisecond)
	runtime.setSnapshot(RuntimeSnapshot{
		Connections: []EngineConnection{
			{
				ID:            "conn-1",
				Source:        "10.0.0.1:2000",
				Destination:   "8.8.8.8:443",
				Proxy:         "jp-01",
				Rule:          "GEOIP,CN",
				UploadBytes:   30,
				DownloadBytes: 40,
				StartTime:     1700000100,
			},
			{
				ID:            "conn-10",
				Source:        "10.0.0.10:3000",
				Destination:   "9.9.9.9:443",
				Proxy:         "sg-01",
				Rule:          "MATCH",
				UploadBytes:   50,
				DownloadBytes: 60,
				StartTime:     1700000200,
			},
		},
		Traffic: EngineTraffic{
			UploadTotal:   180,
			DownloadTotal: 500,
		},
		MemoryBytes: 54321,
	})

	connections := manager.GetConnections()
	if len(connections) != 2 {
		t.Fatalf("expected 2 connections from runtime snapshot, got %d", len(connections))
	}
	if connections[0].ID != "conn-1" || connections[1].ID != "conn-10" {
		t.Fatalf("expected natural sorted runtime connections, got %+v", connections)
	}

	traffic := manager.GetTraffic()
	if traffic.UploadTotal != 180 || traffic.DownloadTotal != 500 {
		t.Fatalf("unexpected refreshed traffic: %+v", traffic)
	}
	if traffic.UploadSpeed <= 0 || traffic.DownloadSpeed <= 0 {
		t.Fatalf("expected positive runtime speeds, got %+v", traffic)
	}

	status := manager.GetStatus()
	if status.MemoryBytes != 54321 {
		t.Fatalf("expected memory from runtime snapshot, got %d", status.MemoryBytes)
	}
}

func TestRuntimeSnapshotRefreshIsThrottled(t *testing.T) {
	runtime := newFakeStatsRuntime("fake-v1")
	manager := NewEngineManager(runtime)
	manager.setStatsRefreshInterval(50 * time.Millisecond)

	runtime.setSnapshot(RuntimeSnapshot{
		Traffic: EngineTraffic{
			UploadTotal:   10,
			DownloadTotal: 20,
		},
		MemoryBytes: 1024,
	})

	if _, err := manager.Start("mixed-port: 7890"); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if runtime.snapshotCalls() != 1 {
		t.Fatalf("expected one snapshot during start, got %d", runtime.snapshotCalls())
	}

	_ = manager.GetStatus()
	_ = manager.GetConnections()
	_ = manager.GetTraffic()
	if runtime.snapshotCalls() != 1 {
		t.Fatalf("expected throttled reads to reuse cache, got %d snapshot calls", runtime.snapshotCalls())
	}

	time.Sleep(60 * time.Millisecond)
	_ = manager.GetStatus()
	if runtime.snapshotCalls() != 2 {
		t.Fatalf("expected snapshot refresh after interval, got %d snapshot calls", runtime.snapshotCalls())
	}
}
