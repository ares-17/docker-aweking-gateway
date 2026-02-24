package gateway

import (
	"os"
	"sync"
	"testing"
)

// ─── TopologicalSort ──────────────────────────────────────────────────────────

func TestTopologicalSort(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		containers []ContainerConfig
		wantOrder  []string
		wantErr    bool
	}{
		{
			name:   "no dependencies",
			target: "app",
			containers: []ContainerConfig{
				{Name: "app", TargetPort: "80"},
			},
			wantOrder: []string{"app"},
		},
		{
			name:   "single dependency",
			target: "app",
			containers: []ContainerConfig{
				{Name: "app", TargetPort: "80", DependsOn: []string{"db"}},
				{Name: "db", TargetPort: "5432"},
			},
			wantOrder: []string{"db", "app"},
		},
		{
			name:   "chain: app → api → db",
			target: "app",
			containers: []ContainerConfig{
				{Name: "app", TargetPort: "80", DependsOn: []string{"api"}},
				{Name: "api", TargetPort: "3000", DependsOn: []string{"db"}},
				{Name: "db", TargetPort: "5432"},
			},
			wantOrder: []string{"db", "api", "app"},
		},
		{
			name:   "diamond: app → [api, worker] → db",
			target: "app",
			containers: []ContainerConfig{
				{Name: "app", TargetPort: "80", DependsOn: []string{"api", "worker"}},
				{Name: "api", TargetPort: "3000", DependsOn: []string{"db"}},
				{Name: "worker", TargetPort: "8080", DependsOn: []string{"db"}},
				{Name: "db", TargetPort: "5432"},
			},
			wantOrder: []string{"db", "api", "worker", "app"},
		},
		{
			name:   "cycle detection",
			target: "a",
			containers: []ContainerConfig{
				{Name: "a", TargetPort: "80", DependsOn: []string{"b"}},
				{Name: "b", TargetPort: "80", DependsOn: []string{"a"}},
			},
			wantErr: true,
		},
		{
			name:   "missing dependency",
			target: "app",
			containers: []ContainerConfig{
				{Name: "app", TargetPort: "80", DependsOn: []string{"missing"}},
			},
			wantErr: true,
		},
		{
			name:   "target not found",
			target: "nonexistent",
			containers: []ContainerConfig{
				{Name: "app", TargetPort: "80"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order, err := TopologicalSort(tt.target, tt.containers)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(order) != len(tt.wantOrder) {
				t.Fatalf("order length = %d, want %d: %v", len(order), len(tt.wantOrder), order)
			}
			for i, name := range tt.wantOrder {
				if order[i] != name {
					t.Errorf("order[%d] = %q, want %q (full: %v)", i, order[i], name, order)
				}
			}
		})
	}
}

// ─── GroupRouter ──────────────────────────────────────────────────────────────

func TestGroupRouter_RoundRobin(t *testing.T) {
	gr := NewGroupRouter()

	t.Run("single member always returns it", func(t *testing.T) {
		group := &GroupConfig{Name: "single", Containers: []string{"a"}}
		for i := 0; i < 10; i++ {
			got := gr.Pick(group)
			if got != "a" {
				t.Errorf("Pick() = %q, want %q", got, "a")
			}
		}
	})

	t.Run("round-robin distribution", func(t *testing.T) {
		group := &GroupConfig{Name: "triple", Containers: []string{"a", "b", "c"}}
		counts := make(map[string]int)
		for i := 0; i < 300; i++ {
			counts[gr.Pick(group)]++
		}
		for _, name := range []string{"a", "b", "c"} {
			if counts[name] != 100 {
				t.Errorf("container %q picked %d times, want 100", name, counts[name])
			}
		}
	})

	t.Run("empty group returns empty", func(t *testing.T) {
		group := &GroupConfig{Name: "empty", Containers: nil}
		got := gr.Pick(group)
		if got != "" {
			t.Errorf("Pick() = %q, want empty", got)
		}
	})

	t.Run("concurrent access is safe", func(t *testing.T) {
		group := &GroupConfig{Name: "concurrent", Containers: []string{"x", "y"}}
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = gr.Pick(group)
			}()
		}
		wg.Wait()
		// No race panic = pass
	})
}

// ─── BuildGroupHostIndex ──────────────────────────────────────────────────────

func TestBuildGroupHostIndex(t *testing.T) {
	cfg := &GatewayConfig{
		Groups: []GroupConfig{
			{Name: "g1", Host: "api.local", Containers: []string{"a"}},
			{Name: "g2", Host: "web.local", Containers: []string{"b"}},
		},
	}

	idx := BuildGroupHostIndex(cfg)

	t.Run("known group host", func(t *testing.T) {
		g, ok := idx["api.local"]
		if !ok {
			t.Fatal("expected api.local in index")
		}
		if g.Name != "g1" {
			t.Errorf("Name = %q, want %q", g.Name, "g1")
		}
	})

	t.Run("unknown host", func(t *testing.T) {
		if _, ok := idx["unknown.local"]; ok {
			t.Error("unknown host should not be in the index")
		}
	})

	t.Run("index size", func(t *testing.T) {
		if len(idx) != 2 {
			t.Errorf("index size = %d, want 2", len(idx))
		}
	})
}

// ─── BuildContainerMap ────────────────────────────────────────────────────────

func TestBuildContainerMap(t *testing.T) {
	cfg := &GatewayConfig{
		Containers: []ContainerConfig{
			{Name: "app1", Host: "app1.local", TargetPort: "80"},
			{Name: "db", TargetPort: "5432"},
		},
	}

	m := BuildContainerMap(cfg)

	if _, ok := m["app1"]; !ok {
		t.Error("expected app1 in map")
	}
	if _, ok := m["db"]; !ok {
		t.Error("expected db in map")
	}
	if _, ok := m["missing"]; ok {
		t.Error("missing should not be in map")
	}
}

// ─── Validate groups ──────────────────────────────────────────────────────────

func TestValidate_Groups(t *testing.T) {
	tests := []struct {
		name    string
		cfg     GatewayConfig
		wantErr bool
	}{
		{
			name: "valid group",
			cfg: GatewayConfig{
				Gateway: GlobalConfig{Port: "8080"},
				Containers: []ContainerConfig{
					{Name: "api-1", TargetPort: "80"},
					{Name: "api-2", TargetPort: "80"},
				},
				Groups: []GroupConfig{
					{Name: "api", Host: "api.local", Strategy: "round-robin", Containers: []string{"api-1", "api-2"}},
				},
			},
			wantErr: false,
		},
		{
			name: "group references unknown container",
			cfg: GatewayConfig{
				Gateway:    GlobalConfig{Port: "8080"},
				Containers: []ContainerConfig{{Name: "api-1", TargetPort: "80"}},
				Groups: []GroupConfig{
					{Name: "api", Host: "api.local", Containers: []string{"api-1", "api-99"}},
				},
			},
			wantErr: true,
		},
		{
			name: "group host conflicts with container host",
			cfg: GatewayConfig{
				Gateway:    GlobalConfig{Port: "8080"},
				Containers: []ContainerConfig{{Name: "app", Host: "app.local", TargetPort: "80"}},
				Groups: []GroupConfig{
					{Name: "g1", Host: "app.local", Containers: []string{"app"}},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate group name",
			cfg: GatewayConfig{
				Gateway:    GlobalConfig{Port: "8080"},
				Containers: []ContainerConfig{{Name: "a", TargetPort: "80"}, {Name: "b", TargetPort: "80"}},
				Groups: []GroupConfig{
					{Name: "g1", Host: "a.local", Containers: []string{"a"}},
					{Name: "g1", Host: "b.local", Containers: []string{"b"}},
				},
			},
			wantErr: true,
		},
		{
			name: "group missing name",
			cfg: GatewayConfig{
				Gateway:    GlobalConfig{Port: "8080"},
				Containers: []ContainerConfig{{Name: "a", TargetPort: "80"}},
				Groups:     []GroupConfig{{Host: "a.local", Containers: []string{"a"}}},
			},
			wantErr: true,
		},
		{
			name: "group with no containers",
			cfg: GatewayConfig{
				Gateway: GlobalConfig{Port: "8080"},
				Groups:  []GroupConfig{{Name: "empty", Host: "e.local"}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ─── Validate depends_on ─────────────────────────────────────────────────────

func TestValidate_DependsOn(t *testing.T) {
	tests := []struct {
		name    string
		cfg     GatewayConfig
		wantErr bool
	}{
		{
			name: "valid depends_on",
			cfg: GatewayConfig{
				Gateway: GlobalConfig{Port: "8080"},
				Containers: []ContainerConfig{
					{Name: "app", Host: "app.local", TargetPort: "80", DependsOn: []string{"db"}},
					{Name: "db", TargetPort: "5432"},
				},
			},
			wantErr: false,
		},
		{
			name: "depends on unknown container",
			cfg: GatewayConfig{
				Gateway: GlobalConfig{Port: "8080"},
				Containers: []ContainerConfig{
					{Name: "app", Host: "app.local", TargetPort: "80", DependsOn: []string{"missing"}},
				},
			},
			wantErr: true,
		},
		{
			name: "self-dependency",
			cfg: GatewayConfig{
				Gateway: GlobalConfig{Port: "8080"},
				Containers: []ContainerConfig{
					{Name: "app", Host: "app.local", TargetPort: "80", DependsOn: []string{"app"}},
				},
			},
			wantErr: true,
		},
		{
			name: "cycle A → B → A",
			cfg: GatewayConfig{
				Gateway: GlobalConfig{Port: "8080"},
				Containers: []ContainerConfig{
					{Name: "a", Host: "a.local", TargetPort: "80", DependsOn: []string{"b"}},
					{Name: "b", Host: "b.local", TargetPort: "80", DependsOn: []string{"a"}},
				},
			},
			wantErr: true,
		},
		{
			name: "dependency container doesn't need host",
			cfg: GatewayConfig{
				Gateway: GlobalConfig{Port: "8080"},
				Containers: []ContainerConfig{
					{Name: "app", Host: "app.local", TargetPort: "80", DependsOn: []string{"db"}},
					{Name: "db", TargetPort: "5432"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ─── applyDefaults for groups ─────────────────────────────────────────────────

func TestApplyDefaults_Groups(t *testing.T) {
	cfg := GatewayConfig{
		Groups: []GroupConfig{
			{Name: "g1", Host: "g.local", Containers: []string{"a"}},
		},
	}
	applyDefaults(&cfg)

	if cfg.Groups[0].Strategy != "round-robin" {
		t.Errorf("Strategy = %q, want %q", cfg.Groups[0].Strategy, "round-robin")
	}
}

func TestApplyDefaults_GroupExplicitStrategy(t *testing.T) {
	cfg := GatewayConfig{
		Groups: []GroupConfig{
			{Name: "g1", Host: "g.local", Strategy: "custom", Containers: []string{"a"}},
		},
	}
	applyDefaults(&cfg)

	if cfg.Groups[0].Strategy != "custom" {
		t.Errorf("Strategy = %q, want %q", cfg.Groups[0].Strategy, "custom")
	}
}

// ─── MergeConfigs preserves DependsOn ─────────────────────────────────────────

func TestMergeConfigs_PreservesDependsOn(t *testing.T) {
	dm := &DiscoveryManager{
		staticConfig: &GatewayConfig{
			Gateway: GlobalConfig{Port: "8080"},
		},
	}

	dynamic := []ContainerConfig{
		{
			Name:       "app",
			Host:       "app.local",
			TargetPort: "80",
			DependsOn:  []string{"db", "redis"},
		},
	}

	merged := dm.mergeConfigs(dynamic)
	if len(merged.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(merged.Containers))
	}
	c := merged.Containers[0]
	if len(c.DependsOn) != 2 {
		t.Fatalf("DependsOn length = %d, want 2", len(c.DependsOn))
	}
	if c.DependsOn[0] != "db" || c.DependsOn[1] != "redis" {
		t.Errorf("DependsOn = %v, want [db redis]", c.DependsOn)
	}
}

// ─── Config loading with groups and depends_on ────────────────────────────────

func TestLoadConfig_GroupsAndDeps(t *testing.T) {
	yamlContent := `
gateway:
  port: "8080"
containers:
  - name: "api-1"
    target_port: "8080"
    depends_on: ["db"]
  - name: "api-2"
    target_port: "8080"
    depends_on: ["db"]
  - name: "db"
    target_port: "5432"
groups:
  - name: "api-cluster"
    host: "api.local"
    containers: ["api-1", "api-2"]
`
	tmp := t.TempDir()
	path := tmp + "/config.yaml"
	if err := writeFile(path, yamlContent); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_PATH", path)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}

	if len(cfg.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(cfg.Groups))
	}
	if cfg.Groups[0].Strategy != "round-robin" {
		t.Errorf("Strategy = %q, want %q", cfg.Groups[0].Strategy, "round-robin")
	}
	if len(cfg.Containers[0].DependsOn) != 1 || cfg.Containers[0].DependsOn[0] != "db" {
		t.Errorf("api-1 DependsOn = %v, want [db]", cfg.Containers[0].DependsOn)
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
