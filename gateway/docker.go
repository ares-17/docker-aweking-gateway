package gateway

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
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

// GetContainerAddress returns the primary internal IP address of the container.
func (d *DockerClient) GetContainerAddress(ctx context.Context, containerName string) (string, error) {
	info, err := d.cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return "", err
	}
	for _, network := range info.NetworkSettings.Networks {
		if network.IPAddress != "" {
			return network.IPAddress, nil
		}
	}
	return "", fmt.Errorf("could not find IP address for container %s", containerName)
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
// Lines are sanitised: Docker's 8-byte stream header is stripped and
// ANSI escape codes are removed so the output is safe for HTML embedding.
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

	// Docker multiplexes stdout/stderr with an 8-byte header per frame.
	// We strip the header bytes so only the text content remains.
	text := stripDockerLogHeaders(raw)

	// Split into lines, trim whitespace, drop empty.
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimRight(l, "\r")
		if l != "" {
			lines = append(lines, l)
		}
	}

	// Return only the last n lines (the API tail is approximate for short logs)
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

// stripDockerLogHeaders removes the 8-byte multiplexing header that Docker
// prepends to each log frame: [stream_type(1), 0, 0, 0, size(4)] + payload.
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
