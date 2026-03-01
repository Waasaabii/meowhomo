package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	pb "github.com/Waasaabii/meowhomo/engine/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	// gRPC 监听地址和可选自动启动配置由环境变量控制。
	address := envOrDefault("MEOWHOMO_ENGINE_ADDR", "127.0.0.1:50051")
	autoStartConfig := strings.TrimSpace(os.Getenv("MEOWHOMO_ENGINE_AUTOSTART_CONFIG"))
	runtimeOptions := MihomoRuntimeOptions{
		HomeDir:                strings.TrimSpace(os.Getenv("MEOWHOMO_ENGINE_HOME")),
		ConfigPath:             strings.TrimSpace(os.Getenv("MEOWHOMO_ENGINE_CONFIG_PATH")),
		ExternalController:     strings.TrimSpace(os.Getenv("MEOWHOMO_ENGINE_EXTERNAL_CONTROLLER")),
		ExternalControllerUnix: strings.TrimSpace(os.Getenv("MEOWHOMO_ENGINE_EXTERNAL_CONTROLLER_UNIX")),
		ExternalControllerPipe: strings.TrimSpace(os.Getenv("MEOWHOMO_ENGINE_EXTERNAL_CONTROLLER_PIPE")),
		ExternalUI:             strings.TrimSpace(os.Getenv("MEOWHOMO_ENGINE_EXTERNAL_UI")),
		Secret:                 strings.TrimSpace(os.Getenv("MEOWHOMO_ENGINE_SECRET")),
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", address, err)
	}

	manager := NewEngineManager(NewMihomoRuntime(runtimeOptions))
	if autoStartConfig != "" {
		if _, startErr := manager.Start(autoStartConfig); startErr != nil {
			log.Fatalf("failed to auto start engine: %v", startErr)
		}
	}
	service := NewEngineGRPCServer(manager)

	grpcServer := grpc.NewServer()
	pb.RegisterMihomoEngineServer(grpcServer, service)
	reflection.Register(grpcServer)

	log.Printf(
		"🐱 MeowHomo Engine gRPC listening on %s (runtime=mihomo, version=%s)",
		address,
		manager.GetStatus().Version,
	)

	go func() {
		if serveErr := grpcServer.Serve(listener); serveErr != nil {
			log.Fatalf("grpc serve error: %v", serveErr)
		}
	}()

	stopSignal := make(chan os.Signal, 1)
	signal.Notify(stopSignal, syscall.SIGINT, syscall.SIGTERM)
	stop := <-stopSignal

	log.Printf("received stop signal %s, shutting down engine gRPC server", stop.String())
	if _, stopErr := manager.Stop(); stopErr != nil && stopErr != ErrEngineNotRunning {
		log.Printf("runtime stop warning: %v", stopErr)
	}
	grpcServer.GracefulStop()
}

// envOrDefault 读取环境变量；当值为空时返回默认值。
func envOrDefault(envName, fallback string) string {
	value := strings.TrimSpace(os.Getenv(envName))
	if value == "" {
		return fallback
	}
	return value
}
