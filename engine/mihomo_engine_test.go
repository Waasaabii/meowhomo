package main

import (
	"errors"
	"testing"
)

func TestMihomoRuntimeBuildHubOptions(t *testing.T) {
	runtime := NewMihomoRuntime(MihomoRuntimeOptions{
		ExternalController:     "127.0.0.1:9090",
		ExternalControllerUnix: "/tmp/mihomo.sock",
		ExternalControllerPipe: "mihomo-pipe",
		ExternalUI:             "./ui",
		Secret:                 "token",
	})

	options := runtime.buildHubOptions()
	if len(options) != 5 {
		t.Fatalf("expected 5 hub options, got %d", len(options))
	}
}

func TestMihomoRuntimeVersion(t *testing.T) {
	runtime := NewMihomoRuntime(MihomoRuntimeOptions{})
	if runtime.Version() == "" {
		t.Fatal("runtime version should not be empty")
	}
}

func TestMihomoRuntimeSupportsLogHandler(t *testing.T) {
	runtime := NewMihomoRuntime(MihomoRuntimeOptions{})

	logRuntime, ok := any(runtime).(EngineLogAwareRuntime)
	if !ok {
		t.Fatal("mihomo runtime should implement EngineLogAwareRuntime")
	}

	logRuntime.SetLogHandler(func(level, message string) {})
}

func TestMihomoRuntimeStopRequiresRunning(t *testing.T) {
	runtime := NewMihomoRuntime(MihomoRuntimeOptions{})
	err := runtime.Stop()
	if !errors.Is(err, ErrEngineNotRunning) {
		t.Fatalf("expected ErrEngineNotRunning, got %v", err)
	}
}

func TestValidateConfigYAML(t *testing.T) {
	validConfig := "mixed-port: 7890\nmode: rule\n"
	if err := validateConfigYAML(validConfig); err != nil {
		t.Fatalf("expected valid yaml, got %v", err)
	}

	invalidSyntax := "mixed-port: 7890\n  mode: rule"
	if err := validateConfigYAML(invalidSyntax); err == nil {
		t.Fatal("expected syntax validation error")
	}

	invalidRoot := "- just-a-list"
	if err := validateConfigYAML(invalidRoot); err == nil {
		t.Fatal("expected mapping root validation error")
	}
}
