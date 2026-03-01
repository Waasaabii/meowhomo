package main

import "sync"

type fakeRuntime struct {
	mu      sync.Mutex
	started bool
	version string
}

func newFakeRuntime(version string) *fakeRuntime {
	if version == "" {
		version = "fake-runtime"
	}
	return &fakeRuntime{version: version}
}

func (runtime *fakeRuntime) Start(configYAML string) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if runtime.started {
		return ErrEngineRunning
	}
	runtime.started = true
	return nil
}

func (runtime *fakeRuntime) Reload(configYAML string) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if !runtime.started {
		return ErrEngineNotRunning
	}
	return nil
}

func (runtime *fakeRuntime) Stop() error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.started = false
	return nil
}

func (runtime *fakeRuntime) Version() string {
	return runtime.version
}

type fakeStatsRuntime struct {
	*fakeRuntime
	mu       sync.Mutex
	snapshot RuntimeSnapshot
	calls    int
}

func newFakeStatsRuntime(version string) *fakeStatsRuntime {
	return &fakeStatsRuntime{
		fakeRuntime: newFakeRuntime(version),
	}
}

func (runtime *fakeStatsRuntime) Snapshot() (RuntimeSnapshot, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	runtime.calls++
	return runtime.snapshot, nil
}

func (runtime *fakeStatsRuntime) setSnapshot(snapshot RuntimeSnapshot) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	runtime.snapshot = snapshot
}

func (runtime *fakeStatsRuntime) snapshotCalls() int {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.calls
}
