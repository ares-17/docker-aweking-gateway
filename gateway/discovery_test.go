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
}
