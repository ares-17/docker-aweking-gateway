package gateway

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	dockernetwork "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

// DockerClient handles interactions with the Docker daemon
type DockerClient struct {
	cli *client.Client
}

// NewDockerClient creates a new DockerClient instance
func NewDockerClient() (*DockerClient, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &DockerClient{cli: cli}, nil
}

// GetContainerStatus returns the status of a container (e.g. "running", "exited")
func (d *DockerClient) GetContainerStatus(ctx context.Context, containerName string) (string, error) {
	info, err := d.cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return "", err
	}
	return info.State.Status, nil
}

// GetContainerAddress returns the IP address of the container.
// If network is non-empty, it looks up that specific Docker network.
// Otherwise it returns the IP from the first available network.
func (d *DockerClient) GetContainerAddress(ctx context.Context, containerName, network string) (string, error) {
	info, err := d.cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return "", err
	}

	nets := info.NetworkSettings.Networks
	if len(nets) == 0 {
		return "", fmt.Errorf("container %s has no network interfaces", containerName)
	}

	// Prefer the requested network if specified
	if network != "" {
		if n, ok := nets[network]; ok && n.IPAddress != "" {
			return n.IPAddress, nil
		}
		return "", fmt.Errorf("container %s is not on network %q (attached networks: %s)",
			containerName, network, joinNetworkNames(nets))
	}

	// Fallback: return the first non-empty IP
	for _, n := range nets {
		if n.IPAddress != "" {
			return n.IPAddress, nil
		}
	}
	return "", fmt.Errorf("could not find IP address for container %s", containerName)
}

// joinNetworkNames lists attached network names for error messages.
func joinNetworkNames(nets map[string]*dockernetwork.EndpointSettings) string {
	names := make([]string, 0, len(nets))
	for name := range nets {
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}

// ProbeTCP attempts a TCP connection to ip:port, retrying every 300 ms until
// the connection succeeds or ctx is cancelled. Returns nil on success.
func (d *DockerClient) ProbeTCP(ctx context.Context, ip, port string) error {
	addr := net.JoinHostPort(ip, port)
	for {
		dialer := &net.Dialer{}
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err == nil {
			conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("TCP probe timed out for %s: %w", addr, ctx.Err())
		case <-time.After(300 * time.Millisecond):
			// retry
		}
	}
}

// StartContainer starts a container by name.
func (d *DockerClient) StartContainer(ctx context.Context, containerName string) error {
	return d.cli.ContainerStart(ctx, containerName, container.StartOptions{})
}

// StopContainer stops a running container gracefully.
func (d *DockerClient) StopContainer(ctx context.Context, containerName string) error {
	return d.cli.ContainerStop(ctx, containerName, container.StopOptions{})
}

// GetContainerLogs returns the last n log lines from the container.
// Lines are sanitised: Docker's 8-byte stream header is stripped and the
// output is safe for rendering as plain text in the browser.
func (d *DockerClient) GetContainerLogs(ctx context.Context, containerName string, n int) ([]string, error) {
	tail := fmt.Sprintf("%d", n)
	opts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
		Timestamps: false,
	}
	rc, err := d.cli.ContainerLogs(ctx, containerName, opts)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	raw, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	text := stripDockerLogHeaders(raw)

	var lines []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimRight(l, "\r")
		if l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

// stripDockerLogHeaders removes the 8-byte multiplexing header Docker prepends
// to each log frame: [stream_type(1), 0, 0, 0, size(4)] + payload.
func stripDockerLogHeaders(b []byte) string {
	var buf bytes.Buffer
	for len(b) >= 8 {
		size := int(b[4])<<24 | int(b[5])<<16 | int(b[6])<<8 | int(b[7])
		b = b[8:]
		if size > len(b) {
			size = len(b)
		}
		buf.Write(b[:size])
		b = b[size:]
	}
	return buf.String()
}

// Close closes the Docker client connection
func (d *DockerClient) Close() error {
	return d.cli.Close()
}
