package main

import (
	"context"
	"log"

	"docker-gateway/gateway"
)

func main() {
	// Load YAML configuration (path from CONFIG_PATH env, default /etc/gateway/config.yaml)
	cfg, err := gateway.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize Docker client
	dockerClient, err := gateway.NewDockerClient()
	if err != nil {
		log.Fatalf("Failed to initialize Docker client: %v", err)
	}
	defer dockerClient.Close()

	// Initialize Container Manager
	manager := gateway.NewContainerManager(dockerClient)

	// Start idle-watcher goroutine (stops containers after their idle_timeout)
	manager.StartIdleWatcher(context.Background(), cfg.Containers)

	// Initialize and start the HTTP server
	server, err := gateway.NewServer(manager, cfg)
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}

	if err := server.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
