package main

import (
	"log"
	"os"

	"docker-gateway/gateway"
)

func main() {
	// Initialize Docker client
	dockerClient, err := gateway.NewDockerClient()
	if err != nil {
		log.Fatalf("Failed to initialize Docker client: %v", err)
	}
	defer dockerClient.Close()

	// Initialize Container Manager
	manager := gateway.NewContainerManager(dockerClient)

	// Initialize Server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server, err := gateway.NewServer(manager, port)
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}

	// Start Server
	if err := server.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
