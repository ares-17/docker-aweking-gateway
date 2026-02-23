package main

import (
	"context"
	"log"

	"os"
	"os/signal"
	"syscall"

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

	// Initialize and start the HTTP server
	server, err := gateway.NewServer(manager, cfg)
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}

	// Start idle-watcher goroutine with a callback to get the latest config
	manager.StartIdleWatcher(context.Background(), func() []gateway.ContainerConfig {
		return server.GetConfig().Containers
	})

	// Setup config hot-reload on SIGHUP
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)
	go func() {
		for range sigChan {
			log.Println("Received SIGHUP, reloading configuration...")
			newCfg, err := gateway.LoadConfig()
			if err != nil {
				log.Printf("Hot-reload failed: %v", err)
				continue
			}
			server.ReloadConfig(newCfg)
			log.Println("Configuration reloaded successfully")
		}
	}()

	if err := server.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
