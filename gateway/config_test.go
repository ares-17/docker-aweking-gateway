package gateway

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ─── applyDefaults ────────────────────────────────────────────────────────────

func TestApplyDefaults(t *testing.T) {
	tests := []struct {
		name   string
		input  GatewayConfig
		check  func(t *testing.T, cfg *GatewayConfig)
	}{
		{
			name:  "all empty → defaults applied",
			input: GatewayConfig{},
			check: func(t *testing.T, cfg *GatewayConfig) {
				if cfg.Gateway.Port != "8080" {
					t.Errorf("Port = %q, want %q", cfg.Gateway.Port, "8080")
				}
				if cfg.Gateway.LogLines != 30 {
					t.Errorf("LogLines = %d, want %d", cfg.Gateway.LogLines, 30)
				}
			},
		},
		{
			name: "explicit values preserved",
			input: GatewayConfig{
				Gateway: GlobalConfig{Port: "9090", LogLines: 50},
			},
			check: func(t *testing.T, cfg *GatewayConfig) {
				if cfg.Gateway.Port != "9090" {
					t.Errorf("Port should not be overridden, got %q", cfg.Gateway.Port)
				}
				if cfg.Gateway.LogLines != 50 {
					t.Errorf("LogLines should not be overridden, got %d", cfg.Gateway.LogLines)
				}
			},
		},
		{
			name: "container defaults applied",
			input: GatewayConfig{
				Containers: []ContainerConfig{
					{Name: "app", Host: "app.local"},
				},
			},
			check: func(t *testing.T, cfg *GatewayConfig) {
				c := cfg.Containers[0]
				if c.TargetPort != "80" {
					t.Errorf("TargetPort = %q, want %q", c.TargetPort, "80")
				}
				if c.StartTimeout != 60*time.Second {
					t.Errorf("StartTimeout = %v, want %v", c.StartTimeout, 60*time.Second)
				}
				if c.RedirectPath != "/" {
					t.Errorf("RedirectPath = %q, want %q", c.RedirectPath, "/")
				}
				if c.Icon != "docker" {
					t.Errorf("Icon = %q, want %q", c.Icon, "docker")
				}
			},
		},
		{
			name: "container explicit values preserved",
			input: GatewayConfig{
				Containers: []ContainerConfig{
					{
						Name:         "app",
						Host:         "app.local",
						TargetPort:   "3000",
						StartTimeout: 120 * time.Second,
						RedirectPath: "/dashboard",
						Icon:         "nginx",
					},
				},
			},
			check: func(t *testing.T, cfg *GatewayConfig) {
				c := cfg.Containers[0]
				if c.TargetPort != "3000" {
					t.Errorf("TargetPort should not be overridden, got %q", c.TargetPort)
				}
				if c.StartTimeout != 120*time.Second {
					t.Errorf("StartTimeout should not be overridden, got %v", c.StartTimeout)
				}
				if c.RedirectPath != "/dashboard" {
					t.Errorf("RedirectPath should not be overridden, got %q", c.RedirectPath)
				}
				if c.Icon != "nginx" {
					t.Errorf("Icon should not be overridden, got %q", c.Icon)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.input
			applyDefaults(&cfg)
			tt.check(t, &cfg)
		})
	}
}

// ─── Validate ─────────────────────────────────────────────────────────────────

func TestValidate(t *testing.T) {
	base := func() GatewayConfig {
		return GatewayConfig{
			Gateway: GlobalConfig{Port: "8080"},
			Containers: []ContainerConfig{
				{Name: "app", Host: "app.local", TargetPort: "80"},
			},
		}
	}

	tests := []struct {
		name    string
		modify  func(cfg *GatewayConfig)
		wantErr bool
	}{
		{
			name:    "valid config",
			modify:  func(cfg *GatewayConfig) {},
			wantErr: false,
		},
		{
			name:    "empty port",
			modify:  func(cfg *GatewayConfig) { cfg.Gateway.Port = "" },
			wantErr: true,
		},
		{
			name: "missing container name",
			modify: func(cfg *GatewayConfig) {
				cfg.Containers[0].Name = ""
			},
			wantErr: true,
		},
		{
			name: "missing container host",
			modify: func(cfg *GatewayConfig) {
				cfg.Containers[0].Host = ""
			},
			wantErr: true,
		},
		{
			name: "missing container target_port",
			modify: func(cfg *GatewayConfig) {
				cfg.Containers[0].TargetPort = ""
			},
			wantErr: true,
		},
		{
			name: "duplicate container name",
			modify: func(cfg *GatewayConfig) {
				cfg.Containers = append(cfg.Containers, ContainerConfig{
					Name: "app", Host: "other.local", TargetPort: "80",
				})
			},
			wantErr: true,
		},
		{
			name: "duplicate host",
			modify: func(cfg *GatewayConfig) {
				cfg.Containers = append(cfg.Containers, ContainerConfig{
					Name: "app2", Host: "app.local", TargetPort: "80",
				})
			},
			wantErr: true,
		},
		{
			name: "multiple valid containers",
			modify: func(cfg *GatewayConfig) {
				cfg.Containers = append(cfg.Containers, ContainerConfig{
					Name: "db", Host: "db.local", TargetPort: "5432",
				})
			},
			wantErr: false,
		},
		{
			name: "zero containers is valid",
			modify: func(cfg *GatewayConfig) {
				cfg.Containers = nil
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base()
			tt.modify(&cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ─── BuildHostIndex ───────────────────────────────────────────────────────────

func TestBuildHostIndex(t *testing.T) {
	cfg := &GatewayConfig{
		Containers: []ContainerConfig{
			{Name: "app1", Host: "app1.local"},
			{Name: "app2", Host: "app2.local"},
			{Name: "no-host", Host: ""},
		},
	}

	idx := BuildHostIndex(cfg)

	t.Run("known host returns correct config", func(t *testing.T) {
		got, ok := idx["app1.local"]
		if !ok {
			t.Fatal("expected app1.local in index")
		}
		if got.Name != "app1" {
			t.Errorf("Name = %q, want %q", got.Name, "app1")
		}
	})

	t.Run("second host returns correct config", func(t *testing.T) {
		got, ok := idx["app2.local"]
		if !ok {
			t.Fatal("expected app2.local in index")
		}
		if got.Name != "app2" {
			t.Errorf("Name = %q, want %q", got.Name, "app2")
		}
	})

	t.Run("empty host not indexed", func(t *testing.T) {
		if _, ok := idx[""]; ok {
			t.Error("empty host should not be in the index")
		}
	})

	t.Run("unknown host returns nil", func(t *testing.T) {
		if _, ok := idx["unknown.local"]; ok {
			t.Error("unknown host should not be in the index")
		}
	})

	t.Run("index size excludes empty hosts", func(t *testing.T) {
		if len(idx) != 2 {
			t.Errorf("index size = %d, want 2", len(idx))
		}
	})
}

// ─── LoadConfig (file-based) ──────────────────────────────────────────────────

func TestLoadConfig_MissingFile(t *testing.T) {
	t.Setenv("CONFIG_PATH", "/nonexistent/file.yaml")
	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "bad.yaml")
	if err := os.WriteFile(path, []byte("{{{{not yaml"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_PATH", path)
	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadConfig_ValidFile(t *testing.T) {
	yaml := `
gateway:
  port: "9090"
  log_lines: 10
containers:
  - name: "test-app"
    host: "test.local"
    target_port: "3000"
    start_timeout: "30s"
`
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_PATH", path)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.Gateway.Port != "9090" {
		t.Errorf("Port = %q, want %q", cfg.Gateway.Port, "9090")
	}
	if cfg.Gateway.LogLines != 10 {
		t.Errorf("LogLines = %d, want %d", cfg.Gateway.LogLines, 10)
	}
	if len(cfg.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(cfg.Containers))
	}
	if cfg.Containers[0].StartTimeout != 30*time.Second {
		t.Errorf("StartTimeout = %v, want %v", cfg.Containers[0].StartTimeout, 30*time.Second)
	}
}

func TestLoadConfig_ValidationFails(t *testing.T) {
	yaml := `
gateway:
  port: "8080"
containers:
  - name: ""
    host: "test.local"
`
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_PATH", path)

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected validation error for empty container name")
	}
}
