package gateway

import (
	"context"
	"log"
	"sync"
	"time"
)

// ContainerManager orchestrates container lifecycle: starting on demand,
// preventing concurrent starts, and auto-stopping idle containers.
type ContainerManager struct {
	client *DockerClient

	// mu protects the locks and lastSeen maps
	mu       sync.Mutex
	locks    map[string]*sync.Mutex
	lastSeen map[string]time.Time
}

func NewContainerManager(client *DockerClient) *ContainerManager {
	return &ContainerManager{
		client:   client,
		locks:    make(map[string]*sync.Mutex),
		lastSeen: make(map[string]time.Time),
	}
}

// getLock returns (or creates) a per-container mutex used to serialise starts.
func (m *ContainerManager) getLock(containerName string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.locks[containerName]; !ok {
		m.locks[containerName] = &sync.Mutex{}
	}
	return m.locks[containerName]
}

// RecordActivity records the current time as the last activity for a container.
// Call this on every successfully proxied request.
func (m *ContainerManager) RecordActivity(containerName string) {
	m.mu.Lock()
	m.lastSeen[containerName] = time.Now()
	m.mu.Unlock()
}

// EnsureRunning checks whether a container is running and, if not, starts it.
// It uses cfg.StartTimeout as the maximum wait time and prevents duplicate starts
// via a per-container mutex. Returns "running" on success or an error.
func (m *ContainerManager) EnsureRunning(ctx context.Context, cfg *ContainerConfig) (string, error) {
	status, err := m.client.GetContainerStatus(ctx, cfg.Name)
	if err != nil {
		return "", err
	}
	if status == "running" {
		return "running", nil
	}

	// Acquire per-container lock to prevent parallel start attempts.
	lock := m.getLock(cfg.Name)
	lock.Lock()
	defer lock.Unlock()

	// Double-check after acquiring lock (another goroutine may have started it).
	status, err = m.client.GetContainerStatus(ctx, cfg.Name)
	if err != nil {
		return "", err
	}
	if status == "running" {
		return "running", nil
	}

	// Start the container.
	if err := m.client.StartContainer(ctx, cfg.Name); err != nil {
		return "", err
	}

	// Poll until running or start_timeout elapses.
	timeoutCtx, cancel := context.WithTimeout(ctx, cfg.StartTimeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return "", timeoutCtx.Err()
		case <-ticker.C:
			status, err := m.client.GetContainerStatus(ctx, cfg.Name)
			if err != nil {
				continue
			}
			if status == "running" {
				return "running", nil
			}
		}
	}
}

// StartIdleWatcher starts a background goroutine that periodically checks
// each container's last activity time. Containers that have exceeded their
// idle_timeout are stopped automatically.
// Containers with IdleTimeout == 0 are never stopped.
func (m *ContainerManager) StartIdleWatcher(ctx context.Context, cfgs []ContainerConfig) {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.checkIdle(ctx, cfgs)
			}
		}
	}()
}

func (m *ContainerManager) checkIdle(ctx context.Context, cfgs []ContainerConfig) {
	m.mu.Lock()
	snapshot := make(map[string]time.Time, len(m.lastSeen))
	for k, v := range m.lastSeen {
		snapshot[k] = v
	}
	m.mu.Unlock()

	now := time.Now()
	for _, cfg := range cfgs {
		if cfg.IdleTimeout == 0 {
			continue
		}
		last, seen := snapshot[cfg.Name]
		if !seen {
			continue // never had traffic, don't auto-stop
		}
		if now.Sub(last) < cfg.IdleTimeout {
			continue
		}

		// Check the container is actually running before stopping.
		status, err := m.client.GetContainerStatus(ctx, cfg.Name)
		if err != nil || status != "running" {
			continue
		}

		log.Printf("idle-watcher: stopping %q (idle for %s)", cfg.Name, now.Sub(last).Round(time.Second))
		if err := m.client.StopContainer(ctx, cfg.Name); err != nil {
			log.Printf("idle-watcher: failed to stop %q: %v", cfg.Name, err)
		}
	}
}
