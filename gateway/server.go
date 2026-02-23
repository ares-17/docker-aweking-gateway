package gateway

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

//go:embed templates/*.html
var templatesFS embed.FS

// Server handles HTTP traffic for the gateway.
type Server struct {
	manager   *ContainerManager
	cfg       *GatewayConfig
	hostIndex map[string]*ContainerConfig // host → config, O(1) lookup
	tmpl      *template.Template
}

func NewServer(manager *ContainerManager, cfg *GatewayConfig) (*Server, error) {
	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	return &Server{
		manager:   manager,
		cfg:       cfg,
		hostIndex: BuildHostIndex(cfg),
		tmpl:      tmpl,
	}, nil
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/_health", s.handleHealth)
	mux.HandleFunc("/_logs", s.handleLogs)
	mux.HandleFunc("/", s.handleRequest)

	srv := &http.Server{
		Addr:         ":" + s.cfg.Gateway.Port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	fmt.Printf("Docker Awakening Gateway listening on :%s\n", s.cfg.Gateway.Port)
	return srv.ListenAndServe()
}

// resolveConfig maps an incoming request to its ContainerConfig by Host header.
func (s *Server) resolveConfig(r *http.Request) *ContainerConfig {
	host := r.Host
	// Exact match first
	if cfg, ok := s.hostIndex[host]; ok {
		return cfg
	}
	// Fallback: strip port and try bare hostname
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		if cfg, ok := s.hostIndex[host[:idx]]; ok {
			return cfg
		}
	}
	// Query-param override (for testing: ?container=my-app)
	if name := r.URL.Query().Get("container"); name != "" {
		for i := range s.cfg.Containers {
			if s.cfg.Containers[i].Name == name {
				return &s.cfg.Containers[i]
			}
		}
	}
	return nil
}

// handleRequest is the main entry point: proxy or serve loading page.
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Skip internal endpoints that arrive here due to mux routing edge cases
	if r.URL.Path == "/_health" || r.URL.Path == "/_logs" {
		http.NotFound(w, r)
		return
	}

	cfg := s.resolveConfig(r)
	if cfg == nil {
		http.Error(w, "No container configured for this host. Check config.yaml.", http.StatusNotFound)
		return
	}

	ctx := r.Context()
	status, err := s.manager.client.GetContainerStatus(ctx, cfg.Name)
	if err != nil {
		if strings.Contains(err.Error(), "No such container") {
			s.serveErrorPage(w, r, cfg, "Container not found in Docker daemon")
		} else {
			s.serveErrorPage(w, r, cfg, fmt.Sprintf("Docker error: %v", err))
		}
		return
	}

	if status == "running" {
		s.manager.RecordActivity(cfg.Name)
		s.proxyRequest(w, r, cfg)
		return
	}

	// Container not running — trigger async start and serve loading page.
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), cfg.StartTimeout+5*time.Second)
		defer cancel()
		if _, err := s.manager.EnsureRunning(bgCtx, cfg); err != nil {
			fmt.Printf("async start error for %q: %v\n", cfg.Name, err)
		}
	}()

	s.serveLoadingPage(w, r, cfg)
}

// handleHealth returns {"status":"<docker-status>"} for polling by the loading page.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	cfg := s.resolveConfig(r)
	if cfg == nil {
		http.Error(w, "unknown container", http.StatusBadRequest)
		return
	}

	status, err := s.manager.client.GetContainerStatus(r.Context(), cfg.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": status})
}

// handleLogs returns {"lines":["..."]} with the last N log lines of the container.
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	cfg := s.resolveConfig(r)
	if cfg == nil {
		http.Error(w, "unknown container", http.StatusBadRequest)
		return
	}

	lines, err := s.manager.client.GetContainerLogs(r.Context(), cfg.Name, s.cfg.Gateway.LogLines)
	if err != nil {
		// Return empty lines rather than an error — the container may be starting.
		lines = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string][]string{"lines": lines})
}

// proxyRequest forwards the HTTP request to the target container.
func (s *Server) proxyRequest(w http.ResponseWriter, r *http.Request, cfg *ContainerConfig) {
	ip, err := s.manager.client.GetContainerAddress(r.Context(), cfg.Name)
	if err != nil {
		s.serveErrorPage(w, r, cfg, fmt.Sprintf("Networking error: %v", err))
		return
	}

	targetURL, _ := url.Parse(fmt.Sprintf("http://%s:%s", ip, cfg.TargetPort))
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	r.URL.Host = targetURL.Host
	r.URL.Scheme = targetURL.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = targetURL.Host

	proxy.ServeHTTP(w, r)
}

// ─── Template data structs ────────────────────────────────────────────────────

type loadingData struct {
	ContainerName string
	RequestID     string
	RequestPath   string
	RedirectPath  string
	StartTimeout  string // human-readable for display
}

type errorData struct {
	ContainerName string
	Error         string
	RequestID     string
	RequestPath   string
}

func requestID(prefix string) string {
	return fmt.Sprintf("%s-%x", prefix, time.Now().UnixNano()%0xFFFFFF)
}

func (s *Server) serveLoadingPage(w http.ResponseWriter, r *http.Request, cfg *ContainerConfig) {
	data := loadingData{
		ContainerName: cfg.Name,
		RequestID:     requestID("req"),
		RequestPath:   r.URL.Path,
		RedirectPath:  cfg.RedirectPath,
		StartTimeout:  cfg.StartTimeout.String(),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "loading.html", data); err != nil {
		fmt.Printf("template error (loading): %v\n", err)
	}
}

func (s *Server) serveErrorPage(w http.ResponseWriter, r *http.Request, cfg *ContainerConfig, errMsg string) {
	data := errorData{
		ContainerName: cfg.Name,
		Error:         errMsg,
		RequestID:     requestID("err"),
		RequestPath:   r.URL.Path,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadGateway)
	if err := s.tmpl.ExecuteTemplate(w, "error.html", data); err != nil {
		fmt.Printf("template error (error): %v\n", err)
	}
}
