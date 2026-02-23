package gateway

import (
	"context"
	"fmt"

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

// GetContainerStatus returns the status of a container (e.g., "running", "exited")
func (d *DockerClient) GetContainerStatus(ctx context.Context, containerName string) (string, error) {
	// Inspect the container to get detailed information
	json, err := d.cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return "", err
	}
	return json.State.Status, nil
}

// GetContainerAddress returns the internal IP address and port of the container.
// For now, it assumes the container exposes a port and we use its primary network IP.
func (d *DockerClient) GetContainerAddress(ctx context.Context, containerName string) (string, error) {
	json, err := d.cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return "", err
	}

	// Try to get IP from the first network found
	for _, network := range json.NetworkSettings.Networks {
		if network.IPAddress != "" {
			// We assume port 80 for now, or we could inspect exposed ports
			return network.IPAddress, nil
		}
	}

	return "", fmt.Errorf("could not find IP address for container %s", containerName)
}

// StartContainer starts a container by name
func (d *DockerClient) StartContainer(ctx context.Context, containerName string) error {
	return d.cli.ContainerStart(ctx, containerName, container.StartOptions{})
}

// Close closes the Docker client connection
func (d *DockerClient) Close() error {
	return d.cli.Close()
}
