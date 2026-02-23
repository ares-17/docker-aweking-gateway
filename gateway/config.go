package gateway

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// GatewayConfig is the top-level config structure parsed from config.yaml
type GatewayConfig struct {
	Gateway    GlobalConfig      `yaml:"gateway"`
	Containers []ContainerConfig `yaml:"containers"`
}

// GlobalConfig holds gateway-wide settings
type GlobalConfig struct {
	// Port the gateway listens on (default: "8080")
	Port string `yaml:"port"`
	// LogLines is the number of container log lines shown in the loading page (default: 30)
	LogLines int `yaml:"log_lines"`
}

// ContainerConfig holds per-container settings
type ContainerConfig struct {
	// Name is the Docker container name to manage
	Name string `yaml:"name"`
	// Host is the incoming Host header to match (e.g. "myapp.localhost")
	Host string `yaml:"host"`
	// TargetPort is the port on the container to proxy to (default: "80")
	TargetPort string `yaml:"target_port"`
	// StartTimeout is the maximum time to wait for the container to start.
	// After this duration the error page is shown. (default: 60s)
	StartTimeout time.Duration `yaml:"start_timeout"`
	// IdleTimeout is how long the container may be idle (no incoming requests)
	// before it is automatically stopped. 0 means never auto-stop. (default: 0)
	IdleTimeout time.Duration `yaml:"idle_timeout"`
	// RedirectPath is the URL path the browser is sent to once the container is
	// running. Useful when the web UI is not at "/". (default: "/")
	RedirectPath string `yaml:"redirect_path"`
}

// LoadConfig reads and parses the YAML config file.
// The path is taken from the CONFIG_PATH env var (default: /etc/gateway/config.yaml).
func LoadConfig() (*GatewayConfig, error) {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "/etc/gateway/config.yaml"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config file %q: %w", path, err)
	}

	var cfg GatewayConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("cannot parse config file %q: %w", path, err)
	}

	applyDefaults(&cfg)
	return &cfg, nil
}

// applyDefaults fills in sensible defaults for any unset field.
func applyDefaults(cfg *GatewayConfig) {
	if cfg.Gateway.Port == "" {
		cfg.Gateway.Port = "8080"
	}
	if cfg.Gateway.LogLines == 0 {
		cfg.Gateway.LogLines = 30
	}

	for i := range cfg.Containers {
		c := &cfg.Containers[i]
		if c.TargetPort == "" {
			c.TargetPort = "80"
		}
		if c.StartTimeout == 0 {
			c.StartTimeout = 60 * time.Second
		}
		// IdleTimeout 0 means "never auto-stop" — no default override needed
		if c.RedirectPath == "" {
			c.RedirectPath = "/"
		}
	}
}

// BuildHostIndex returns a map from Host header value → ContainerConfig for O(1) lookup.
func BuildHostIndex(cfg *GatewayConfig) map[string]*ContainerConfig {
	idx := make(map[string]*ContainerConfig, len(cfg.Containers))
	for i := range cfg.Containers {
		if cfg.Containers[i].Host != "" {
			idx[cfg.Containers[i].Host] = &cfg.Containers[i]
		}
	}
	return idx
}
