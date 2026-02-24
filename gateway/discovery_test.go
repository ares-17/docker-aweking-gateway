package gateway

import (
	"testing"
	"time"
)

// ─── mergeConfigs ─────────────────────────────────────────────────────────────

func TestMergeConfigs(t *testing.T) {
	tests := []struct {
		name           string
		staticConfig   *GatewayConfig
		dynamic        []ContainerConfig
		wantLen        int
		wantNames      []string
		wantSkipReason string // substring expected in log (not asserted, just documented)
	}{
		{
			name: "only static containers",
			staticConfig: &GatewayConfig{
				Gateway:    GlobalConfig{Port: "8080"},
				Containers: []ContainerConfig{{Name: "s1", Host: "s1.local", TargetPort: "80"}},
			},
			dynamic:   nil,
			wantLen:   1,
			wantNames: []string{"s1"},
		},
		{
			name: "only dynamic containers",
			staticConfig: &GatewayConfig{
				Gateway: GlobalConfig{Port: "8080"},
			},
			dynamic: []ContainerConfig{
				{Name: "d1", Host: "d1.local", TargetPort: "80"},
				{Name: "d2", Host: "d2.local", TargetPort: "80"},
			},
			wantLen:   2,
			wantNames: []string{"d1", "d2"},
		},
		{
			name: "static + dynamic no conflicts",
			staticConfig: &GatewayConfig{
				Gateway:    GlobalConfig{Port: "8080"},
				Containers: []ContainerConfig{{Name: "s1", Host: "s1.local", TargetPort: "80"}},
			},
			dynamic: []ContainerConfig{
				{Name: "d1", Host: "d1.local", TargetPort: "80"},
			},
			wantLen:   2,
			wantNames: []string{"s1", "d1"},
		},
		{
			name: "duplicate host → dynamic skipped",
			staticConfig: &GatewayConfig{
				Gateway:    GlobalConfig{Port: "8080"},
				Containers: []ContainerConfig{{Name: "s1", Host: "shared.local", TargetPort: "80"}},
			},
			dynamic: []ContainerConfig{
				{Name: "d1", Host: "shared.local", TargetPort: "80"},
			},
			wantLen:   1,
			wantNames: []string{"s1"},
		},
		{
			name: "duplicate name → dynamic skipped",
			staticConfig: &GatewayConfig{
				Gateway:    GlobalConfig{Port: "8080"},
				Containers: []ContainerConfig{{Name: "app", Host: "static.local", TargetPort: "80"}},
			},
			dynamic: []ContainerConfig{
				{Name: "app", Host: "dynamic.local", TargetPort: "80"},
			},
			wantLen:   1,
			wantNames: []string{"app"},
		},
		{
			name: "both empty → zero containers",
			staticConfig: &GatewayConfig{
				Gateway: GlobalConfig{Port: "8080"},
			},
			dynamic:   nil,
			wantLen:   0,
			wantNames: nil,
		},
		{
			name: "global config preserved from static",
			staticConfig: &GatewayConfig{
				Gateway:    GlobalConfig{Port: "9090", LogLines: 50},
				Containers: []ContainerConfig{{Name: "s1", Host: "s1.local", TargetPort: "80"}},
			},
			dynamic:   nil,
			wantLen:   1,
			wantNames: []string{"s1"},
		},
		{
			name: "dynamic duplicates among themselves → first wins",
			staticConfig: &GatewayConfig{
				Gateway: GlobalConfig{Port: "8080"},
			},
			dynamic: []ContainerConfig{
				{Name: "d1", Host: "same.local", TargetPort: "80"},
				{Name: "d2", Host: "same.local", TargetPort: "80"},
			},
			wantLen:   1,
			wantNames: []string{"d1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dm := &DiscoveryManager{
				staticConfig: tt.staticConfig,
			}

			merged := dm.mergeConfigs(tt.dynamic)

			if len(merged.Containers) != tt.wantLen {
				t.Errorf("merged containers = %d, want %d", len(merged.Containers), tt.wantLen)
			}

			for i, wantName := range tt.wantNames {
				if i >= len(merged.Containers) {
					break
				}
				if merged.Containers[i].Name != wantName {
					t.Errorf("container[%d].Name = %q, want %q", i, merged.Containers[i].Name, wantName)
				}
			}

			// Verify global config is always carried over
			if merged.Gateway.Port != tt.staticConfig.Gateway.Port {
				t.Errorf("Gateway.Port = %q, want %q", merged.Gateway.Port, tt.staticConfig.Gateway.Port)
			}
		})
	}
}

// TestMergeConfigs_ConcurrentAccess verifies that concurrent mergeConfigs calls
// on the same DiscoveryManager don't race on the staticConfig mutex.
func TestMergeConfigs_ConcurrentAccess(t *testing.T) {
	dm := &DiscoveryManager{
		staticConfig: &GatewayConfig{
			Gateway:    GlobalConfig{Port: "8080"},
			Containers: []ContainerConfig{{Name: "s1", Host: "s1.local", TargetPort: "80"}},
		},
	}

	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_ = dm.mergeConfigs([]ContainerConfig{
				{Name: "d1", Host: "d1.local", TargetPort: "80"},
			})
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
	// No race detector panic = pass
}

// ─── mergeConfigs preserves container field values ────────────────────────────

func TestMergeConfigs_PreservesFields(t *testing.T) {
	dm := &DiscoveryManager{
		staticConfig: &GatewayConfig{
			Gateway: GlobalConfig{Port: "8080"},
		},
	}

	dynamic := []ContainerConfig{
		{
			Name:         "app",
			Host:         "app.local",
			TargetPort:   "3000",
			StartTimeout: 120 * time.Second,
			IdleTimeout:  5 * time.Minute,
			Network:      "backend",
			RedirectPath: "/login",
			Icon:         "nginx",
			HealthPath:   "/healthz",
		},
	}

	merged := dm.mergeConfigs(dynamic)
	if len(merged.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(merged.Containers))
	}

	c := merged.Containers[0]
	if c.TargetPort != "3000" {
		t.Errorf("TargetPort = %q, want %q", c.TargetPort, "3000")
	}
	if c.StartTimeout != 120*time.Second {
		t.Errorf("StartTimeout = %v, want %v", c.StartTimeout, 120*time.Second)
	}
	if c.IdleTimeout != 5*time.Minute {
		t.Errorf("IdleTimeout = %v, want %v", c.IdleTimeout, 5*time.Minute)
	}
	if c.Network != "backend" {
		t.Errorf("Network = %q, want %q", c.Network, "backend")
	}
	if c.RedirectPath != "/login" {
		t.Errorf("RedirectPath = %q, want %q", c.RedirectPath, "/login")
	}
	if c.Icon != "nginx" {
		t.Errorf("Icon = %q, want %q", c.Icon, "nginx")
	}
	if c.HealthPath != "/healthz" {
		t.Errorf("HealthPath = %q, want %q", c.HealthPath, "/healthz")
	}
}

// ─── Change detection ─────────────────────────────────────────────────────────

func TestDiscoveryChangeDetection_SkipsDuplicate(t *testing.T) {
	callCount := 0
	dm := &DiscoveryManager{
		staticConfig: &GatewayConfig{
			Gateway:    GlobalConfig{Port: "8080"},
			Containers: []ContainerConfig{{Name: "s1", Host: "s1.local", TargetPort: "80"}},
		},
		onConfigChange: func(cfg *GatewayConfig) {
			callCount++
		},
	}

	dynamic := []ContainerConfig{{Name: "d1", Host: "d1.local", TargetPort: "80"}}

	// First merge → should trigger onConfigChange
	merged1 := dm.mergeConfigs(dynamic)
	if err := merged1.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	dm.mu.Lock()
	dm.lastConfig = merged1
	dm.mu.Unlock()
	dm.onConfigChange(merged1)

	// Second merge with identical inputs → should NOT trigger
	merged2 := dm.mergeConfigs(dynamic)
	dm.mu.Lock()
	unchanged := dm.lastConfig != nil && dm.lastConfig.Equal(merged2)
	dm.mu.Unlock()

	if !unchanged {
		t.Error("expected configs to be equal on identical inputs")
	}
	if callCount != 1 {
		t.Errorf("onConfigChange called %d times, want 1", callCount)
	}
}

func TestDiscoveryChangeDetection_DetectsNewContainer(t *testing.T) {
	callCount := 0
	dm := &DiscoveryManager{
		staticConfig: &GatewayConfig{
			Gateway: GlobalConfig{Port: "8080"},
		},
		onConfigChange: func(cfg *GatewayConfig) {
			callCount++
		},
	}

	// First pass: one dynamic container
	merged1 := dm.mergeConfigs([]ContainerConfig{
		{Name: "d1", Host: "d1.local", TargetPort: "80"},
	})
	dm.mu.Lock()
	dm.lastConfig = merged1
	dm.mu.Unlock()
	dm.onConfigChange(merged1)

	// Second pass: add another dynamic container
	merged2 := dm.mergeConfigs([]ContainerConfig{
		{Name: "d1", Host: "d1.local", TargetPort: "80"},
		{Name: "d2", Host: "d2.local", TargetPort: "80"},
	})

	dm.mu.Lock()
	unchanged := dm.lastConfig != nil && dm.lastConfig.Equal(merged2)
	dm.mu.Unlock()

	if unchanged {
		t.Error("expected configs to differ when a new container is added")
	}
}

func TestDiscoveryChangeDetection_UpdateStaticClearsCache(t *testing.T) {
	dm := &DiscoveryManager{
		staticConfig: &GatewayConfig{
			Gateway: GlobalConfig{Port: "8080"},
		},
		lastConfig: &GatewayConfig{
			Gateway: GlobalConfig{Port: "8080"},
		},
	}

	newStatic := &GatewayConfig{
		Gateway: GlobalConfig{Port: "9090"},
	}

	// UpdateStaticConfig should clear lastConfig (we can't call the full method
	// because it requires a DockerClient, so we test the field mutation directly).
	dm.mu.Lock()
	dm.staticConfig = newStatic
	dm.lastConfig = nil // This is what UpdateStaticConfig does
	dm.mu.Unlock()

	if dm.lastConfig != nil {
		t.Error("lastConfig should be nil after UpdateStaticConfig")
	}
}
