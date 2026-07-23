// Package service provides Docker management business logic.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/jR4dh3y/BoxBox/backend/internal/model"
)

// DockerService handles Docker operations.
type DockerService struct {
	client *client.Client
}

// DockerServiceConfig holds configuration for DockerService.
type DockerServiceConfig struct {
	SocketPath string // e.g., /var/run/docker.sock
	Host       string // e.g., tcp://192.168.1.100:2375 (overrides SocketPath)
}

// NewDockerService creates a new Docker service.
func NewDockerService(cfg DockerServiceConfig) (*DockerService, error) {
	opts := []client.Opt{
		client.WithAPIVersionNegotiation(),
	}

	// Use TCP host if provided, otherwise use socket
	if cfg.Host != "" {
		opts = append(opts, client.WithHost(cfg.Host))
	} else if cfg.SocketPath != "" {
		opts = append(opts, client.WithHost("unix://"+cfg.SocketPath))
	}

	c, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &DockerService{client: c}, nil
}

// ListContainers returns all containers with their status.
func (s *DockerService) ListContainers(ctx context.Context) ([]model.Container, error) {
	containers, err := s.client.ContainerList(ctx, container.ListOptions{
		All: true, // Include stopped containers
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	result := make([]model.Container, 0, len(containers))
	for _, c := range containers {
		container := s.convertContainer(c)
		result = append(result, container)
	}

	return result, nil
}

// GetContainer returns a single container by ID.
func (s *DockerService) GetContainer(ctx context.Context, id string) (*model.Container, error) {
	info, err := s.client.ContainerInspect(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	created, _ := time.Parse(time.RFC3339Nano, info.Created)
	container := model.Container{
		ID:      info.ID[:12],
		Name:    strings.TrimPrefix(info.Name, "/"),
		Image:   info.Config.Image,
		Status:  info.State.Status,
		State:   info.State.Status,
		Created: created,
		Labels:  info.Config.Labels,
	}

	// Convert port bindings
	for port, bindings := range info.NetworkSettings.Ports {
		for _, binding := range bindings {
			container.Ports = append(container.Ports, model.PortBinding{
				HostIP:        binding.HostIP,
				HostPort:      binding.HostPort,
				ContainerPort: string(port),
				Protocol:      port.Proto(),
			})
		}
	}

	return &container, nil
}

// StartContainer starts a container.
func (s *DockerService) StartContainer(ctx context.Context, id string) error {
	return s.client.ContainerStart(ctx, id, container.StartOptions{})
}

// StopContainer stops a container.
func (s *DockerService) StopContainer(ctx context.Context, id string) error {
	timeout := 10 // seconds
	return s.client.ContainerStop(ctx, id, container.StopOptions{
		Timeout: &timeout,
	})
}

// RestartContainer restarts a container.
func (s *DockerService) RestartContainer(ctx context.Context, id string) error {
	timeout := 10
	return s.client.ContainerRestart(ctx, id, container.StopOptions{
		Timeout: &timeout,
	})
}

// DeleteContainer removes a container.
func (s *DockerService) DeleteContainer(ctx context.Context, id string) error {
	return s.client.ContainerRemove(ctx, id, container.RemoveOptions{
		Force: true,
	})
}

// GetContainerLogs returns container logs.
func (s *DockerService) GetContainerLogs(ctx context.Context, id string, tail int) ([]string, error) {
	if tail <= 0 {
		tail = 100
	}

	reader, err := s.client.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       fmt.Sprintf("%d", tail),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get container logs: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read logs: %w", err)
	}

	// Docker multiplexed stream format: 8-byte header + payload
	// For simplicity, we'll split by newlines
	logs := strings.Split(string(data), "\n")
	return logs, nil
}

// GetStats returns container resource usage statistics.
func (s *DockerService) GetStats(ctx context.Context, id string) (cpu float64, memory model.MemoryUsage, err error) {
	statsReader, err := s.client.ContainerStats(ctx, id, false)
	if err != nil {
		return 0, model.MemoryUsage{}, fmt.Errorf("failed to get container stats: %w", err)
	}
	defer statsReader.Body.Close()

	var stats types.StatsJSON
	if err := json.NewDecoder(statsReader.Body).Decode(&stats); err != nil {
		return 0, model.MemoryUsage{}, fmt.Errorf("failed to decode stats: %w", err)
	}

	// Calculate CPU percentage
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)
	if systemDelta > 0 && cpuDelta > 0 {
		cpu = (cpuDelta / systemDelta) * float64(stats.CPUStats.OnlineCPUs) * 100.0
	}

	// Calculate memory usage
	memory = model.MemoryUsage{
		Usage:   int64(stats.MemoryStats.Usage),
		Limit:   int64(stats.MemoryStats.Limit),
		Percent: float64(stats.MemoryStats.Usage) / float64(stats.MemoryStats.Limit) * 100.0,
	}

	return cpu, memory, nil
}

// ListImages returns all Docker images.
func (s *DockerService) ListImages(ctx context.Context) ([]types.ImageSummary, error) {
	return s.client.ImageList(ctx, types.ImageListOptions{})
}

// DeleteImage removes a Docker image.
func (s *DockerService) DeleteImage(ctx context.Context, id string) error {
	_, err := s.client.ImageRemove(ctx, id, types.ImageRemoveOptions{})
	return err
}

// GetDockerStats returns system-wide Docker statistics.
func (s *DockerService) GetDockerStats(ctx context.Context) (*model.DockerStats, error) {
	stats := &model.DockerStats{}

	// Get containers
	containers, err := s.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	stats.Containers.Total = len(containers)
	for _, c := range containers {
		switch c.State {
		case "running":
			stats.Containers.Running++
		case "exited":
			stats.Containers.Stopped++
		case "paused":
			stats.Containers.Paused++
		}
	}

	// Get images
	images, err := s.client.ImageList(ctx, types.ImageListOptions{})
	if err != nil {
		return nil, err
	}
	stats.Images.Total = len(images)
	for _, img := range images {
		stats.Images.Size += img.Size
	}

	return stats, nil
}

// ListComposeProjects finds and returns Docker Compose projects.
func (s *DockerService) ListComposeProjects(ctx context.Context, paths []string) ([]model.ComposeProject, error) {
	projects := make([]model.ComposeProject, 0)

	for _, searchPath := range paths {
		// Find docker-compose files in the path
		cmd := exec.Command("find", searchPath, "-maxdepth", "2", "-name", "docker-compose.yml", "-o", "-name", "docker-compose.yaml", "-o", "-name", "compose.yml", "-o", "-name", "compose.yaml")
		output, err := cmd.Output()
		if err != nil {
			continue // Skip paths that can't be accessed
		}

		files := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, file := range files {
			if file == "" {
				continue
			}

			// Extract project name from directory
			parts := strings.Split(file, "/")
			if len(parts) < 2 {
				continue
			}
			projectName := parts[len(parts)-2]
			projectPath := strings.TrimSuffix(file, "/"+parts[len(parts)-1])

			project := model.ComposeProject{
				ID:   projectName,
				Name: projectName,
				Path: projectPath,
			}

			// Try to read compose file
			if content, err := readFileContent(file); err == nil {
				project.File = content
			}

			// Try to read .env file
			envPath := strings.TrimSuffix(file, filepath.Ext(file)) + ".env"
			if content, err := readFileContent(envPath); err == nil {
				project.EnvFile = content
			}

			projects = append(projects, project)
		}
	}

	return projects, nil
}

// ComposeUp runs docker-compose up.
func (s *DockerService) ComposeUp(ctx context.Context, projectPath string) (*model.ComposeAction, error) {
	cmd := exec.CommandContext(ctx, "docker-compose", "up", "-d")
	cmd.Dir = projectPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return &model.ComposeAction{
			Success: false,
			Message: "Failed to start compose project",
			Output:  string(output),
		}, err
	}

	return &model.ComposeAction{
		Success: true,
		Message: "Compose project started",
		Output:  string(output),
	}, nil
}

// ComposeDown runs docker-compose down.
func (s *DockerService) ComposeDown(ctx context.Context, projectPath string) (*model.ComposeAction, error) {
	cmd := exec.CommandContext(ctx, "docker-compose", "down")
	cmd.Dir = projectPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return &model.ComposeAction{
			Success: false,
			Message: "Failed to stop compose project",
			Output:  string(output),
		}, err
	}

	return &model.ComposeAction{
		Success: true,
		Message: "Compose project stopped",
		Output:  string(output),
	}, nil
}

// ComposeBuild runs docker-compose build.
func (s *DockerService) ComposeBuild(ctx context.Context, projectPath string) (*model.ComposeAction, error) {
	cmd := exec.CommandContext(ctx, "docker-compose", "build")
	cmd.Dir = projectPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return &model.ComposeAction{
			Success: false,
			Message: "Failed to build compose project",
			Output:  string(output),
		}, err
	}

	return &model.ComposeAction{
		Success: true,
		Message: "Compose project built",
		Output:  string(output),
	}, nil
}

// ComposeRestart runs docker-compose restart.
func (s *DockerService) ComposeRestart(ctx context.Context, projectPath string) (*model.ComposeAction, error) {
	cmd := exec.CommandContext(ctx, "docker-compose", "restart")
	cmd.Dir = projectPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return &model.ComposeAction{
			Success: false,
			Message: "Failed to restart compose project",
			Output:  string(output),
		}, err
	}

	return &model.ComposeAction{
		Success: true,
		Message: "Compose project restarted",
		Output:  string(output),
	}, nil
}

// ComposePull runs docker-compose pull.
func (s *DockerService) ComposePull(ctx context.Context, projectPath string) (*model.ComposeAction, error) {
	cmd := exec.CommandContext(ctx, "docker-compose", "pull")
	cmd.Dir = projectPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return &model.ComposeAction{
			Success: false,
			Message: "Failed to pull compose project",
			Output:  string(output),
		}, err
	}

	return &model.ComposeAction{
		Success: true,
		Message: "Compose project pulled",
		Output:  string(output),
	}, nil
}

// ComposeLogs returns docker-compose logs.
func (s *DockerService) ComposeLogs(ctx context.Context, projectPath string, tail int) ([]string, error) {
	if tail <= 0 {
		tail = 100
	}

	cmd := exec.CommandContext(ctx, "docker-compose", "logs", "--tail", fmt.Sprintf("%d", tail))
	cmd.Dir = projectPath

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get compose logs: %w", err)
	}

	return strings.Split(string(output), "\n"), nil
}

// GetComposeFile reads a docker-compose file.
func (s *DockerService) GetComposeFile(projectPath string) (string, error) {
	return readFileContent(projectPath + "/docker-compose.yml")
}

// SaveComposeFile saves a docker-compose file.
func (s *DockerService) SaveComposeFile(projectPath string, content string) error {
	return writeFileContent(projectPath+"/docker-compose.yml", content)
}

// GetComposeEnv reads a .env file.
func (s *DockerService) GetComposeEnv(projectPath string) (string, error) {
	return readFileContent(projectPath + "/.env")
}

// SaveComposeEnv saves a .env file.
func (s *DockerService) SaveComposeEnv(projectPath string, content string) error {
	return writeFileContent(projectPath+"/.env", content)
}

// CreateExec creates a new exec instance in a container.
func (s *DockerService) CreateExec(ctx context.Context, containerID string, cmd []string) (string, error) {
	execConfig := types.ExecConfig{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          cmd,
	}

	r, err := s.client.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create exec: %w", err)
	}
	return r.ID, nil
}

// StartExec attaches to an exec instance and returns the I/O streams.
func (s *DockerService) StartExec(ctx context.Context, execID string) (types.HijackedResponse, error) {
	return s.client.ContainerExecAttach(ctx, execID, types.ExecStartCheck{Tty: true})
}

// convertContainer converts Docker API container to our model.
func (s *DockerService) convertContainer(c types.Container) model.Container {
	container := model.Container{
		ID:      c.ID[:12],
		Name:    strings.TrimPrefix(c.Names[0], "/"),
		Image:   c.Image,
		Status:  c.Status,
		State:   c.State,
		Created: time.Unix(c.Created, 0),
		Labels:  c.Labels,
	}

	// Convert port bindings
	for _, port := range c.Ports {
		container.Ports = append(container.Ports, model.PortBinding{
			HostIP:        port.IP,
			HostPort:      fmt.Sprintf("%d", port.PublicPort),
			ContainerPort: fmt.Sprintf("%d", port.PrivatePort),
			Protocol:      port.Type,
		})
	}

	return container
}

// writeFileContent writes content to a file.
func writeFileContent(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// Helper functions for file operations
func readFileContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// PullImage pulls a Docker image.
func (s *DockerService) PullImage(ctx context.Context, imageRef string) error {
	reader, err := s.client.ImagePull(ctx, imageRef, types.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer reader.Close()

	// Consume the reader to complete the pull
	io.Copy(io.Discard, reader)
	return nil
}

// PruneImages removes unused Docker images.
func (s *DockerService) PruneImages(ctx context.Context) (types.ImagesPruneReport, error) {
	return s.client.ImagesPrune(ctx, filters.NewArgs())
}

// KillContainer sends a signal to a container.
func (s *DockerService) KillContainer(ctx context.Context, id string, signal string) error {
	return s.client.ContainerKill(ctx, id, signal)
}

// ListNetworks returns all Docker networks.
func (s *DockerService) ListNetworks(ctx context.Context) ([]model.Network, error) {
	networks, err := s.client.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}

	result := make([]model.Network, 0, len(networks))
	for _, n := range networks {
		result = append(result, model.Network{
			ID:       n.ID[:12],
			Name:     n.Name,
			Driver:   n.Driver,
			Scope:    n.Scope,
			Created:  n.Created.String(),
			Subnet:   getSubnet(n),
			Gateway:  getGateway(n),
			Internal: n.Internal,
		})
	}

	return result, nil
}

// RemoveNetwork removes a Docker network.
func (s *DockerService) RemoveNetwork(ctx context.Context, id string) error {
	return s.client.NetworkRemove(ctx, id)
}

// PruneNetworks removes unused Docker networks.
func (s *DockerService) PruneNetworks(ctx context.Context) (int64, error) {
	report, err := s.client.NetworksPrune(ctx, filters.NewArgs())
	if err != nil {
		return 0, fmt.Errorf("failed to prune networks: %w", err)
	}
	return int64(len(report.NetworksDeleted)), nil
}

func getSubnet(n types.NetworkResource) string {
	for _, ipam := range n.IPAM.Config {
		if ipam.Subnet != "" {
			return ipam.Subnet
		}
	}
	return ""
}

func getGateway(n types.NetworkResource) string {
	for _, ipam := range n.IPAM.Config {
		if ipam.Gateway != "" {
			return ipam.Gateway
		}
	}
	return ""
}
