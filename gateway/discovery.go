package gateway

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// DiscoveryManager periodically queries Docker for labeled containers
// and merges them with the static configuration.
type DiscoveryManager struct {
	client         *DockerClient
	onConfigChange func(*GatewayConfig)

	mu           sync.Mutex
	staticConfig *GatewayConfig
	lastConfig   *GatewayConfig // last config pushed via onConfigChange
}

// NewDiscoveryManager creates a new discovery engine.
func NewDiscoveryManager(client *DockerClient, staticConfig *GatewayConfig, onConfigChange func(*GatewayConfig)) *DiscoveryManager {
	return &DiscoveryManager{
		client:         client,
		staticConfig:   staticConfig,
		onConfigChange: onConfigChange,
	}
}

// UpdateStaticConfig updates the base static config used during merging,
// typically called after a SIGHUP hot-reload.
// It clears the cached lastConfig to force a reload on the next discovery pass.
func (dm *DiscoveryManager) UpdateStaticConfig(cfg *GatewayConfig) {
	dm.mu.Lock()
	dm.staticConfig = cfg
	dm.lastConfig = nil // force reload
	dm.mu.Unlock()

	// Trigger an immediate discovery pass with the new static config
	dm.runDiscovery(context.Background())
}

// Start begins the polling loop for continuously discovering containers.
func (dm *DiscoveryManager) Start(ctx context.Context, interval time.Duration) {
	// Run once immediately on startup
	dm.runDiscovery(ctx)

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				dm.runDiscovery(ctx)
			}
		}
	}()
}

// runDiscovery executes a single discovery pass
func (dm *DiscoveryManager) runDiscovery(ctx context.Context) {
	dynamicContainers, err := dm.client.DiscoverLabeledContainers(ctx)
	if err != nil {
		slog.Error("discovery: failed to list labeled containers", "error", err)
		return
	}

	merged := dm.mergeConfigs(dynamicContainers)

	// Ensure the merged configuration is valid before pushing it
	if err := merged.Validate(); err != nil {
		slog.Warn("discovery: merge resulted in invalid configuration", "error", err)
		return
	}

	// Only trigger a reload when the config actually changed.
	dm.mu.Lock()
	unchanged := dm.lastConfig != nil && dm.lastConfig.Equal(merged)
	if !unchanged {
		dm.lastConfig = merged
	}
	dm.mu.Unlock()

	if unchanged {
		slog.Debug("discovery: config unchanged, skipping reload")
		return
	}

	dm.onConfigChange(merged)
}

// mergeConfigs safely combines the static config with dynamic discoveries
func (dm *DiscoveryManager) mergeConfigs(dynamic []ContainerConfig) *GatewayConfig {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Copy the static global config
	merged := &GatewayConfig{
		Gateway: dm.staticConfig.Gateway,
	}

	seenHosts := make(map[string]bool)
	seenNames := make(map[string]bool)

	// 1. Add static containers (highest priority)
	for _, sc := range dm.staticConfig.Containers {
		merged.Containers = append(merged.Containers, sc)
		seenHosts[sc.Host] = true
		seenNames[sc.Name] = true
	}

	// 2. Add dynamically discovered containers avoiding conflicts
	for _, dc := range dynamic {
		if seenHosts[dc.Host] {
			slog.Debug("discovery: skipping dynamic container, host already defined", "container", dc.Name, "host", dc.Host)
			continue
		}
		if seenNames[dc.Name] {
			slog.Debug("discovery: skipping dynamic container, name already defined", "container", dc.Name)
			continue
		}
		merged.Containers = append(merged.Containers, dc)
		seenHosts[dc.Host] = true
		seenNames[dc.Name] = true
	}

	return merged
}
