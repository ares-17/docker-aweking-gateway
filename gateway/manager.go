package gateway

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// startStatus represents the lifecycle state of a container start attempt.
type startStatus string

const (
	statusStarting startStatus = "starting"
	statusRunning  startStatus = "running"
	statusFailed   startStatus = "failed"
)

// startState holds the current state of a container start attempt.
type startState struct {
	Status startStatus
	Err    string
}

// ContainerManager orchestrates container lifecycle: starting on demand,
// preventing concurrent starts, and auto-stopping idle containers.
type ContainerManager struct {
	client *DockerClient

	mu          sync.Mutex
	locks       map[string]*sync.Mutex
	lastSeen    map[string]time.Time
	startStates map[string]*startState
}

func NewContainerManager(client *DockerClient) *ContainerManager {
	return &ContainerManager{
		client:      client,
		locks:       make(map[string]*sync.Mutex),
		lastSeen:    make(map[string]time.Time),
		startStates: make(map[string]*startState),
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

// setStartState updates the start state for a container (thread-safe).
func (m *ContainerManager) setStartState(name string, status startStatus, errMsg string) {
	m.mu.Lock()
	m.startStates[name] = &startState{Status: status, Err: errMsg}
	m.mu.Unlock()
}

// GetStartState returns the current start state for a container.
// It is used by the server's /_health endpoint.
func (m *ContainerManager) GetStartState(name string) (status string, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.startStates[name]
	if !ok {
		return "unknown", ""
	}
	return string(s.Status), s.Err
}

// InitStartState marks a container as "starting" before the async goroutine
// fires. This prevents the first /_health poll from returning "unknown".
func (m *ContainerManager) InitStartState(name string) {
	m.setStartState(name, statusStarting, "")
}

// RecordActivity records the current time as the last activity for a container.
// Call this on every successfully proxied request.
func (m *ContainerManager) RecordActivity(containerName string) {
	m.mu.Lock()
	m.lastSeen[containerName] = time.Now()
	m.mu.Unlock()
}

// EnsureRunning checks whether a container is running and, if not, starts it.
// Flow: docker start → wait for "running" state → TCP probe → mark ready.
// Uses cfg.StartTimeout as the total budget for the entire sequence.
func (m *ContainerManager) EnsureRunning(ctx context.Context, cfg *ContainerConfig) error {
	// Check current Docker status
	status, err := m.client.GetContainerStatus(ctx, cfg.Name)
	if err != nil {
		m.setStartState(cfg.Name, statusFailed, fmt.Sprintf("inspect error: %v", err))
		return err
	}
	if status == "running" {
		// Already running — probe TCP to ensure the app is actually listening
		return m.probeTCPReady(ctx, cfg)
	}

	// Acquire per-container lock to prevent parallel start attempts.
	lock := m.getLock(cfg.Name)
	lock.Lock()
	defer lock.Unlock()

	// Double-check after acquiring lock.
	status, err = m.client.GetContainerStatus(ctx, cfg.Name)
	if err != nil {
		m.setStartState(cfg.Name, statusFailed, fmt.Sprintf("inspect error: %v", err))
		return err
	}
	if status == "running" {
		return m.probeTCPReady(ctx, cfg)
	}

	// Start the container.
	m.setStartState(cfg.Name, statusStarting, "")
	if err := m.client.StartContainer(ctx, cfg.Name); err != nil {
		msg := fmt.Sprintf("docker start failed: %v", err)
		m.setStartState(cfg.Name, statusFailed, msg)
		return fmt.Errorf("%s", msg)
	}

	// Poll until Docker reports "running" or start_timeout elapses.
	timeoutCtx, cancel := context.WithTimeout(ctx, cfg.StartTimeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			msg := fmt.Sprintf("start timeout after %s", cfg.StartTimeout)
			m.setStartState(cfg.Name, statusFailed, msg)
			return fmt.Errorf("%s", msg)
		case <-ticker.C:
			status, err := m.client.GetContainerStatus(ctx, cfg.Name)
			if err != nil {
				continue
			}
			if status == "running" {
				// TCP probe with remaining budget
				return m.probeTCPReady(timeoutCtx, cfg)
			}
			if status == "exited" || status == "dead" {
				msg := fmt.Sprintf("container exited unexpectedly (status=%s)", status)
				m.setStartState(cfg.Name, statusFailed, msg)
				return fmt.Errorf("%s", msg)
			}
		}
	}
}

// probeTCPReady probes ip:port until the app responds or ctx expires.
func (m *ContainerManager) probeTCPReady(ctx context.Context, cfg *ContainerConfig) error {
	ip, err := m.client.GetContainerAddress(ctx, cfg.Name, cfg.Network)
	if err != nil {
		msg := fmt.Sprintf("cannot resolve container address: %v", err)
		m.setStartState(cfg.Name, statusFailed, msg)
		return fmt.Errorf("%s", msg)
	}
	if err := m.client.ProbeTCP(ctx, ip, cfg.TargetPort); err != nil {
		msg := fmt.Sprintf("app not responding on port %s: %v", cfg.TargetPort, err)
		m.setStartState(cfg.Name, statusFailed, msg)
		return fmt.Errorf("%s", msg)
	}
	m.setStartState(cfg.Name, statusRunning, "")
	return nil
}

// StartIdleWatcher starts a background goroutine that periodically checks
// each container's last activity time. Containers with IdleTimeout > 0
// that have been idle longer than their timeout are stopped automatically.
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
			continue
		}
		if now.Sub(last) < cfg.IdleTimeout {
			continue
		}
		status, err := m.client.GetContainerStatus(ctx, cfg.Name)
		if err != nil || status != "running" {
			continue
		}
		log.Printf("idle-watcher: stopping %q (idle for %s)", cfg.Name, now.Sub(last).Round(time.Second))
		if err := m.client.StopContainer(ctx, cfg.Name); err != nil {
			log.Printf("idle-watcher: failed to stop %q: %v", cfg.Name, err)
		} else {
			// Reset start state so next request triggers a fresh start
			m.mu.Lock()
			delete(m.startStates, cfg.Name)
			m.mu.Unlock()
		}
	}
}
