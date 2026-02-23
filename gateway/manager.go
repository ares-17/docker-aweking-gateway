package gateway

import (
	"context"
	"sync"
	"time"
)

type ContainerManager struct {
	client *DockerClient
	locks  map[string]*sync.Mutex
	mu     sync.Mutex // Protects the locks map
}

func NewContainerManager(client *DockerClient) *ContainerManager {
	return &ContainerManager{
		client: client,
		locks:  make(map[string]*sync.Mutex),
	}
}

// getLock returns or creates a mutex for a specific container
func (m *ContainerManager) getLock(containerName string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.locks[containerName]; !exists {
		m.locks[containerName] = &sync.Mutex{}
	}
	return m.locks[containerName]
}

// EnsureRunning checks if a container is running, and if not, starts it.
// It handles concurrency so only one start command is issued per container.
func (m *ContainerManager) EnsureRunning(ctx context.Context, containerName string) (string, error) {
	status, err := m.client.GetContainerStatus(ctx, containerName)
	if err != nil {
		return "", err
	}

	if status == "running" {
		return "running", nil
	}

	// Container is not running, acquire lock to start
	lock := m.getLock(containerName)
	lock.Lock()
	defer lock.Unlock()

	// Double-check status after acquiring lock
	status, err = m.client.GetContainerStatus(ctx, containerName)
	if err != nil {
		return "", err
	}
	if status == "running" {
		return "running", nil
	}

	// Start the container
	if err := m.client.StartContainer(ctx, containerName); err != nil {
		return "", err
	}

	// Wait for it to be running (simple polling with timeout)
	// We can refine this later with event listening
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return "", timeoutCtx.Err()
		case <-ticker.C:
			status, err := m.client.GetContainerStatus(ctx, containerName)
			if err != nil {
				continue 
			}
			if status == "running" {
				return "running", nil
			}
		}
	}
}
