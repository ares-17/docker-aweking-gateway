package gateway

import (
	"context"
	"log"
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
func (dm *DiscoveryManager) UpdateStaticConfig(cfg *GatewayConfig) {
	dm.mu.Lock()
	dm.staticConfig = cfg
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
		log.Printf("discovery: failed to list labeled containers: %v", err)
		return
	}

	merged := dm.mergeConfigs(dynamicContainers)

	// Ensure the merged configuration is valid before pushing it
	if err := merged.Validate(); err != nil {
		log.Printf("discovery: merge resulted in invalid configuration: %v", err)
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
			log.Printf("discovery: skipping dynamic container %q because host %q is already defined statically", dc.Name, dc.Host)
			continue
		}
		if seenNames[dc.Name] {
			log.Printf("discovery: skipping dynamic container %q because it is already defined statically", dc.Name)
			continue
		}
		merged.Containers = append(merged.Containers, dc)
		seenHosts[dc.Host] = true
		seenNames[dc.Name] = true
	}

	return merged
}
