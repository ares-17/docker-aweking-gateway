package gateway

import (
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"time"

	"gopkg.in/yaml.v3"
)

// Equal reports whether two GatewayConfig values are semantically identical.
// Used by DiscoveryManager to skip no-op config reloads.
func (c *GatewayConfig) Equal(other *GatewayConfig) bool {
	if c == nil || other == nil {
		return c == other
	}
	return reflect.DeepEqual(c, other)
}

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
	// TrustedProxies is a list of CIDR blocks (e.g. "10.0.0.0/8") whose
	// X-Forwarded-For header is trusted for rate-limiting purposes.
	// If empty, the gateway always uses RemoteAddr. (default: [])
	TrustedProxies []string `yaml:"trusted_proxies"`
	// DiscoveryInterval controls how often Docker labels are polled for
	// auto-discovery. Overridable via DISCOVERY_INTERVAL env var. (default: 15s)
	DiscoveryInterval time.Duration `yaml:"discovery_interval"`
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
	// Network is an optional Docker network name. When set, GetContainerAddress
	// will look up the container IP on this specific network. If empty, the
	// first available network is used. (default: "")
	Network string `yaml:"network"`
	// RedirectPath is the URL path the browser is sent to once the container is
	// running. Useful when the web UI is not at "/". (default: "/")
	RedirectPath string `yaml:"redirect_path"`
	// Icon is an optional Simple Icons slug (e.g. "nginx", "redis", "postgresql").
	// Displayed on the /_status dashboard card. See https://simpleicons.org
	// for available slugs. (default: "docker")
	Icon string `yaml:"icon"`
	// HealthPath is an optional HTTP endpoint (e.g. "/health") called instead
	// of a raw TCP dial to confirm container readiness. When empty the gateway
	// falls back to a TCP probe. (default: "")
	HealthPath string `yaml:"health_path"`
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

	// Allow DISCOVERY_INTERVAL env var to override the YAML / default value.
	if envInterval := os.Getenv("DISCOVERY_INTERVAL"); envInterval != "" {
		if d, err := time.ParseDuration(envInterval); err == nil {
			cfg.Gateway.DiscoveryInterval = d
		} else {
			slog.Warn("invalid DISCOVERY_INTERVAL env var, using default", "value", envInterval, "error", err)
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// Validate checks if the loaded configuration is valid.
func (c *GatewayConfig) Validate() error {
	if c.Gateway.Port == "" {
		return fmt.Errorf("gateway.port cannot be empty")
	}

	seenNames := make(map[string]bool)
	seenHosts := make(map[string]bool)

	for i, ctr := range c.Containers {
		if ctr.Name == "" {
			return fmt.Errorf("container #%d is missing required field 'name'", i+1)
		}
		if ctr.Host == "" {
			return fmt.Errorf("container %q is missing required field 'host'", ctr.Name)
		}
		if ctr.TargetPort == "" {
			return fmt.Errorf("container %q is missing required field 'target_port'", ctr.Name)
		}

		if seenNames[ctr.Name] {
			return fmt.Errorf("duplicate container name found: %q", ctr.Name)
		}
		seenNames[ctr.Name] = true

		if seenHosts[ctr.Host] {
			return fmt.Errorf("duplicate host mapped: %q (in container %q)", ctr.Host, ctr.Name)
		}
		seenHosts[ctr.Host] = true
	}

	return nil
}

// applyDefaults fills in sensible defaults for any unset field.
func applyDefaults(cfg *GatewayConfig) {
	if cfg.Gateway.Port == "" {
		cfg.Gateway.Port = "8080"
	}
	if cfg.Gateway.LogLines == 0 {
		cfg.Gateway.LogLines = 30
	}
	if cfg.Gateway.DiscoveryInterval == 0 {
		cfg.Gateway.DiscoveryInterval = 15 * time.Second
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
		if c.Icon == "" {
			c.Icon = "docker"
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
