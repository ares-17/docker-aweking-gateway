package gateway

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

//go:embed templates/*.html
var templatesFS embed.FS

type Server struct {
	manager *ContainerManager
	port    string
	tmpl    *template.Template
}

func NewServer(manager *ContainerManager, port string) (*Server, error) {
	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	return &Server{
		manager: manager,
		port:    port,
		tmpl:    tmpl,
	}, nil
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/_health", s.handleHealthCheck)
	mux.HandleFunc("/", s.handleRequest)

	server := &http.Server{
		Addr:         ":" + s.port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	fmt.Printf("Docker Awakening Gateway listening on port %s\n", s.port)
	return server.ListenAndServe()
}

func (s *Server) resolveContainerName(r *http.Request) string {
	// 1. Check query param (useful for testing)
	if name := r.URL.Query().Get("container"); name != "" {
		return name
	}

	// 2. Resolve from Host header (e.g., "my-app.localhost" -> "my-app")
	host := r.Host
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}
	
	// If it's a subdomain like app.example.com, we might want "app"
	// For simplicity, we'll try the full host or the first part
	parts := strings.Split(host, ".")
	if len(parts) > 0 && parts[0] != "localhost" {
		return parts[0]
	}

	return ""
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	containerName := s.resolveContainerName(r)
	if containerName == "" {
		http.Error(w, "Could not resolve target container from Host or query param", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	status, err := s.manager.client.GetContainerStatus(ctx, containerName)
	if err != nil {
		if strings.Contains(err.Error(), "No such container") {
			s.serveErrorPage(w, r, containerName, "Container not found in Docker daemon")
			return
		}
		s.serveErrorPage(w, r, containerName, fmt.Sprintf("Docker error: %v", err))
		return
	}

	if status == "running" {
		s.proxyRequest(w, r, containerName)
		return
	}

	// If not running, trigger start asynchronously
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		_, err := s.manager.EnsureRunning(bgCtx, containerName)
		if err != nil {
			fmt.Printf("Error starting container %s: %v\n", containerName, err)
		}
	}()

	// Serve the loading page
	s.serveLoadingPage(w, r, containerName)
}

func (s *Server) proxyRequest(w http.ResponseWriter, r *http.Request, containerName string) {
	ip, err := s.manager.client.GetContainerAddress(r.Context(), containerName)
	if err != nil {
		s.serveErrorPage(w, r, containerName, fmt.Sprintf("Networking error: %v", err))
		return
	}

	// Default to port 80 if not specified (could be made configurable via labels)
	targetPort := os.Getenv("TARGET_PORT")
	if targetPort == "" {
		targetPort = "80"
	}

	targetURL, _ := url.Parse(fmt.Sprintf("http://%s:%s", ip, targetPort))
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	
	// Update the headers to allow for SSL redirection and proper host passing
	r.URL.Host = targetURL.Host
	r.URL.Scheme = targetURL.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = targetURL.Host

	proxy.ServeHTTP(w, r)
}

func (s *Server) serveLoadingPage(w http.ResponseWriter, r *http.Request, containerName string) {
	data := struct {
		ContainerName string
		RequestID     string
		RequestPath   string
	}{
		ContainerName: containerName,
		RequestID:     fmt.Sprintf("req-%x", time.Now().UnixNano()%0xFFFFFF),
		RequestPath:   r.URL.Path,
	}

	w.Header().Set("Content-Type", "text/html")
	if err := s.tmpl.ExecuteTemplate(w, "loading.html", data); err != nil {
		fmt.Printf("Template execution error: %v\n", err)
	}
}

func (s *Server) serveErrorPage(w http.ResponseWriter, r *http.Request, containerName string, errMsg string) {
	data := struct {
		ContainerName string
		Error         string
		RequestID     string
		RequestPath   string
	}{
		ContainerName: containerName,
		Error:         errMsg,
		RequestID:     fmt.Sprintf("err-%x", time.Now().UnixNano()%0xFFFFFF),
		RequestPath:   r.URL.Path,
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusInternalServerError)
	if err := s.tmpl.ExecuteTemplate(w, "error.html", data); err != nil {
		fmt.Printf("Error template execution error: %v\n", err)
	}
}

func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	containerName := s.resolveContainerName(r)
	if containerName == "" {
		http.Error(w, "Missing container identifier", http.StatusBadRequest)
		return
	}

	status, err := s.manager.client.GetContainerStatus(r.Context(), containerName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf(`{"status": "%s"}`, status)))
}
