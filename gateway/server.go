package gateway

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

const gatewayVersion = "0.3.0"

//go:embed templates/*.html
var templatesFS embed.FS

// Server handles HTTP traffic for the gateway.
type Server struct {
	manager     *ContainerManager
	configMu    sync.RWMutex
	cfg         *GatewayConfig
	hostIndex   map[string]*ContainerConfig
	tmpl        *template.Template
	rateLimiter *rateLimiter
}

func NewServer(manager *ContainerManager, cfg *GatewayConfig) (*Server, error) {
	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	return &Server{
		manager:     manager,
		cfg:         cfg,
		hostIndex:   BuildHostIndex(cfg),
		tmpl:        tmpl,
		rateLimiter: newRateLimiter(1 * time.Second),
	}, nil
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/_health", s.handleHealth)
	mux.HandleFunc("/_logs", s.handleLogs)
	mux.HandleFunc("/_status", s.handleStatusPage)
	mux.HandleFunc("/_status/api", s.handleStatusAPI)
	mux.HandleFunc("/_status/wake", s.handleStatusWake)
	mux.HandleFunc("/", s.handleRequest)

	srv := &http.Server{
		Addr:         ":" + s.GetConfig().Gateway.Port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	fmt.Printf("Docker Awakening Gateway v%s listening on :%s\n", gatewayVersion, s.GetConfig().Gateway.Port)
	return srv.ListenAndServe()
}

// ─── Config Hot-Reload ────────────────────────────────────────────────────────

// ReloadConfig safely swaps the active configuration.
func (s *Server) ReloadConfig(newCfg *GatewayConfig) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	s.cfg = newCfg
	s.hostIndex = BuildHostIndex(newCfg)
}

// GetConfig safely retrieves the current configuration.
func (s *Server) GetConfig() *GatewayConfig {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	return s.cfg
}

// ─── Request routing ──────────────────────────────────────────────────────────

// resolveConfig maps an incoming request to its ContainerConfig by Host header.
func (s *Server) resolveConfig(r *http.Request) *ContainerConfig {
	s.configMu.RLock()
	defer s.configMu.RUnlock()

	host := r.Host
	if cfg, ok := s.hostIndex[host]; ok {
		return cfg
	}
	// Strip port and retry
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		if cfg, ok := s.hostIndex[host[:idx]]; ok {
			return cfg
		}
	}
	// Query-param fallback for testing: ?container=my-app
	if name := r.URL.Query().Get("container"); name != "" {
		for i := range s.cfg.Containers {
			if s.cfg.Containers[i].Name == name {
				return &s.cfg.Containers[i]
			}
		}
	}
	return nil
}

// ─── Main handler ─────────────────────────────────────────────────────────────

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/_health" || r.URL.Path == "/_logs" || strings.HasPrefix(r.URL.Path, "/_status") {
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

	// Container not running — pre-set state and trigger async start
	s.manager.InitStartState(cfg.Name)
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), cfg.StartTimeout+10*time.Second)
		defer cancel()
		if err := s.manager.EnsureRunning(bgCtx, cfg); err != nil {
			fmt.Printf("async start error for %q: %v\n", cfg.Name, err)
		}
	}()

	s.serveLoadingPage(w, r, cfg)
}

// ─── Internal endpoints ───────────────────────────────────────────────────────

// handleHealth returns {"status":"starting"|"running"|"failed","error":"..."}.
// The loading page JS polls this to know when to redirect or show inline error.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimiter.Allow(clientIP(r)) {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	cfg := s.resolveConfig(r)
	if cfg == nil {
		http.Error(w, "unknown container", http.StatusBadRequest)
		return
	}

	status, errMsg := s.manager.GetStartState(cfg.Name)

	// If no start attempt recorded yet, fall back to Docker status
	if status == "unknown" {
		dockerStatus, err := s.manager.client.GetContainerStatus(r.Context(), cfg.Name)
		if err == nil && dockerStatus == "running" {
			status = "running"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": status,
		"error":  errMsg,
	})
}

// handleLogs returns {"lines":["..."]} with the last N log lines.
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimiter.Allow(clientIP(r)) {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	cfg := s.resolveConfig(r)
	if cfg == nil {
		http.Error(w, "unknown container", http.StatusBadRequest)
		return
	}

	lines, err := s.manager.client.GetContainerLogs(r.Context(), cfg.Name, s.cfg.Gateway.LogLines)
	if err != nil {
		lines = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string][]string{"lines": lines})
}

// ─── Proxy ────────────────────────────────────────────────────────────────────

// isWebSocketRequest returns true if the request is a WebSocket upgrade.
func isWebSocketRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// proxyRequest forwards an HTTP (or WebSocket) request to the target container.
func (s *Server) proxyRequest(w http.ResponseWriter, r *http.Request, cfg *ContainerConfig) {
	ip, err := s.manager.client.GetContainerAddress(r.Context(), cfg.Name, cfg.Network)
	if err != nil {
		s.serveErrorPage(w, r, cfg, fmt.Sprintf("Networking error: %v", err))
		return
	}

	addr := fmt.Sprintf("%s:%s", ip, cfg.TargetPort)

	if isWebSocketRequest(r) {
		s.proxyWebSocket(w, r, addr)
		return
	}

	targetURL, _ := url.Parse("http://" + addr)
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Pass client IP information to the backend
	setForwardedHeaders(r, ip)

	r.URL.Host = targetURL.Host
	r.URL.Scheme = targetURL.Scheme
	r.Host = targetURL.Host

	proxy.ServeHTTP(w, r)
}

// proxyWebSocket tunnels a WebSocket upgrade through a raw TCP connection.
// It hijacks the client conn and opens a new TCP connection to the backend,
// then copies bidirectionally.
func (s *Server) proxyWebSocket(w http.ResponseWriter, r *http.Request, backendAddr string) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket proxying not supported by this server", http.StatusInternalServerError)
		return
	}

	backend, err := net.DialTimeout("tcp", backendAddr, 10*time.Second)
	if err != nil {
		http.Error(w, fmt.Sprintf("WebSocket backend unreachable: %v", err), http.StatusBadGateway)
		return
	}
	defer backend.Close()

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer clientConn.Close()

	// Forward the original upgrade request to the backend
	if err := r.Write(backend); err != nil {
		return
	}

	// Bidirectional copy until one side closes
	done := make(chan struct{}, 2)
	copy := func(dst io.Writer, src io.Reader) {
		io.Copy(dst, src) //nolint:errcheck
		done <- struct{}{}
	}
	go copy(backend, clientConn)
	go copy(clientConn, backend)
	<-done
}

// setForwardedHeaders adds X-Forwarded-For, X-Real-IP and X-Forwarded-Proto
// to the outgoing request so the backend can see the original client IP.
func setForwardedHeaders(r *http.Request, serverIP string) {
	clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)

	// X-Forwarded-For: append our client IP to any existing chain
	if prior := r.Header.Get("X-Forwarded-For"); prior != "" {
		r.Header.Set("X-Forwarded-For", prior+", "+clientIP)
	} else {
		r.Header.Set("X-Forwarded-For", clientIP)
	}

	// X-Real-IP: the original client (not set if already present upstream)
	if r.Header.Get("X-Real-IP") == "" {
		r.Header.Set("X-Real-IP", clientIP)
	}

	// X-Forwarded-Proto
	proto := "http"
	if r.TLS != nil {
		proto = "https"
	}
	r.Header.Set("X-Forwarded-Proto", proto)
	r.Header.Set("X-Forwarded-Host", r.Host)
}

// clientIP extracts the client IP from the request, respecting X-Forwarded-For.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

// ─── Rate limiter ─────────────────────────────────────────────────────────────

// rateLimiter enforces a minimum interval between requests per IP.
type rateLimiter struct {
	mu          sync.Mutex
	lastSeen    map[string]time.Time
	minInterval time.Duration
}

func newRateLimiter(minInterval time.Duration) *rateLimiter {
	return &rateLimiter{
		lastSeen:    make(map[string]time.Time),
		minInterval: minInterval,
	}
}

// Allow returns true if this IP is allowed to proceed (not rate-limited).
func (rl *rateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	last, ok := rl.lastSeen[ip]
	if !ok || time.Since(last) >= rl.minInterval {
		rl.lastSeen[ip] = time.Now()
		return true
	}
	return false
}

// ─── Template data structs ────────────────────────────────────────────────────

type loadingData struct {
	ContainerName string
	RequestID     string
	RequestPath   string
	RedirectPath  string
	StartTimeout  string
}

type errorData struct {
	ContainerName string
	Error         string
	RequestID     string
	RequestPath   string
}

type statusPageData struct {
	Version string
}

type statusContainerJSON struct {
	Name         string  `json:"name"`
	Host         string  `json:"host"`
	Status       string  `json:"status"`
	StartState   string  `json:"start_state"`
	Image        string  `json:"image"`
	Icon         string  `json:"icon"`
	TargetPort   string  `json:"target_port"`
	StartTimeout string  `json:"start_timeout"`
	IdleTimeout  string  `json:"idle_timeout"`
	StartedAt    *string `json:"started_at,omitempty"`
	LastRequest  *string `json:"last_request,omitempty"`
	Network      string  `json:"network"`
}

type statusAPIResponse struct {
	Containers []statusContainerJSON `json:"containers"`
	UpdatedAt  string                `json:"updated_at"`
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

// ─── Status dashboard handlers ────────────────────────────────────────────────

// handleStatusPage serves the status dashboard HTML page.
func (s *Server) handleStatusPage(w http.ResponseWriter, r *http.Request) {
	data := statusPageData{
		Version: gatewayVersion,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "status.html", data); err != nil {
		fmt.Printf("template error (status): %v\n", err)
		http.Error(w, "Failed to render status page", http.StatusInternalServerError)
	}
}

// handleStatusAPI returns a JSON snapshot of all managed containers.
// Polled every ~5s by the status dashboard JS.
func (s *Server) handleStatusAPI(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimiter.Allow(clientIP(r)) {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	ctx := r.Context()
	cfg := s.GetConfig()
	result := statusAPIResponse{
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
		Containers: make([]statusContainerJSON, 0, len(cfg.Containers)),
	}

	for i := range cfg.Containers {
		c := &cfg.Containers[i]
		entry := statusContainerJSON{
			Name:         c.Name,
			Host:         c.Host,
			Icon:         c.Icon,
			TargetPort:   c.TargetPort,
			StartTimeout: c.StartTimeout.String(),
			IdleTimeout:  c.IdleTimeout.String(),
			Network:      c.Network,
		}

		// Gateway-level start state
		startState, _ := s.manager.GetStartState(c.Name)
		entry.StartState = startState

		// Docker inspect for live status + image + timestamps
		info, err := s.manager.client.InspectContainer(ctx, c.Name)
		if err != nil {
			entry.Status = "unknown"
			entry.Image = "?"
		} else {
			entry.Status = info.Status
			entry.Image = info.Image
			if !info.StartedAt.IsZero() {
				ts := info.StartedAt.UTC().Format(time.RFC3339)
				entry.StartedAt = &ts
			}
		}

		// Last request from in-memory activity tracker
		if t, ok := s.manager.GetLastSeen(c.Name); ok {
			ts := t.UTC().Format(time.RFC3339)
			entry.LastRequest = &ts
		}

		result.Containers = append(result.Containers, entry)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleStatusWake triggers a container start from the dashboard.
func (s *Server) handleStatusWake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.rateLimiter.Allow(clientIP(r)) {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	name := r.URL.Query().Get("container")
	if name == "" {
		http.Error(w, "missing container parameter", http.StatusBadRequest)
		return
	}

	cfg := s.GetConfig()
	var targetCfg *ContainerConfig
	for i := range cfg.Containers {
		if cfg.Containers[i].Name == name {
			targetCfg = &cfg.Containers[i]
			break
		}
	}
	if targetCfg == nil {
		http.Error(w, "unknown container", http.StatusBadRequest)
		return
	}

	// Trigger async start
	s.manager.InitStartState(targetCfg.Name)
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), targetCfg.StartTimeout+10*time.Second)
		defer cancel()
		if err := s.manager.EnsureRunning(bgCtx, targetCfg); err != nil {
			fmt.Printf("status-wake: start error for %q: %v\n", targetCfg.Name, err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
