// Package service provides Docker management business logic.
package service

import (
	"bytes"
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
	"strconv"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
		"github.com/docker/cli/cli/connhelper"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"dockerbox/backend/internal/model"
)

// newSFTPClient creates an SFTP client over SSH.
func (s *DockerService) newSFTPClient() (*sftp.Client, error) {
	if s.sshKey == "" || s.sshHost == "" {
		return nil, fmt.Errorf("no SSH config for SFTP: key=%d host=%s", len(s.sshKey), s.sshHost)
	}

	// Parse host
	host := strings.TrimPrefix(s.sshHost, "ssh://")
	parts := strings.SplitN(host, ":", 2)
	addr := parts[0]
	port := "22"
	if len(parts) > 1 {
		port = parts[1]
	}

	// Parse private key
	signer, err := ssh.ParsePrivateKey([]byte(s.sshKey))
	if err != nil {
		return nil, fmt.Errorf("parse SSH key: %w", err)
	}

	config := &ssh.ClientConfig{
		User:            strings.Split(addr, "@")[0],
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	sshAddr := strings.SplitN(addr, "@", 2)
	if len(sshAddr) > 1 {
		addr = sshAddr[1]
	}
	conn, err := ssh.Dial("tcp", addr+":"+port, config)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s:%s user=%s: %w", addr, port, config.User, err)
	}

	return sftp.NewClient(conn)
}

// sshExec runs a command on the remote host via SSH.
func (s *DockerService) sshExec(ctx context.Context, cmd string) (string, error) {
	if s.sshKey == "" || s.sshHost == "" {
		return "", fmt.Errorf("no SSH config")
	}

	// Write key to temp file
	keyFile, err := os.CreateTemp("", "boxbox-ssh-*")
	if err != nil {
		return "", err
	}
	keyFile.WriteString(s.sshKey)
	keyFile.Close()
	os.Chmod(keyFile.Name(), 0600)
	defer os.Remove(keyFile.Name())

	// Parse host: ssh://user@host:port -> user@host -p port
	host := strings.TrimPrefix(s.sshHost, "ssh://")
	parts := strings.SplitN(host, ":", 2)
	addr := parts[0]
	port := "22"
	if len(parts) > 1 {
		port = parts[1]
	}

	sshCmd := exec.CommandContext(ctx, "ssh",
		"-i", keyFile.Name(),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-p", port,
		addr,
		cmd,
	)
	var stdout, stderr bytes.Buffer
	sshCmd.Stdout = &stdout
	sshCmd.Stderr = &stderr
	err = sshCmd.Run()
	if err != nil {
		return "", fmt.Errorf("ssh exec failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// composeCommand returns the compose command for the given runtime.
// For podman: uses "podman compose" with PODMAN_COMPOSE_WARNING_LOGS=false to suppress wrapper message.
// For docker: uses "docker compose".
func composeCommand(ctx context.Context, runtime string, args ...string) *exec.Cmd {
	if runtime == "podman" {
		cmd := exec.CommandContext(ctx, "podman", append([]string{"compose"}, args...)...)
		cmd.Env = append(os.Environ(), "PODMAN_COMPOSE_WARNING_LOGS=false")
		return cmd
	}
	return exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)
}

// DockerService handles Docker operations.
type DockerService struct {
	client  *client.Client
	sshKey  string // SSH private key content
	sshHost string // user@host:port
	runtime string // "docker" or "podman"
}

// Client returns the underlying Docker API client.
func (s *DockerService) Client() *client.Client {
	return s.client
}

// SSHExec runs a command on the remote host via SSH and returns stdout.
func (s *DockerService) SSHExec(ctx context.Context, cmd string) (string, error) {
	return s.sshExec(ctx, cmd)
}

// DockerServiceConfig holds configuration for DockerService.
type DockerServiceConfig struct {
	SocketPath string // e.g., /var/run/docker.sock
	Host       string // e.g., tcp://192.168.1.100:2375 (overrides SocketPath)
	SSHKey     string // path to SSH private key for ssh:// connections
}

// NewDockerService creates a new Docker service.
func NewDockerService(cfg DockerServiceConfig) (*DockerService, error) {
	opts := []client.Opt{
		client.WithAPIVersionNegotiation(),
	}

	// Use TCP/SSH host if provided, otherwise use socket
	if cfg.Host != "" {
		if strings.HasPrefix(cfg.Host, "ssh://") {
			// SSH connection - use connhelper
			helper, err := connhelper.GetConnectionHelper(cfg.Host)
			if err != nil {
				return nil, fmt.Errorf("failed to create SSH connection helper: %w", err)
			}
			opts = append(opts, client.WithHost(helper.Host), client.WithDialContext(helper.Dialer))
		} else {
			opts = append(opts, client.WithHost(cfg.Host))
		}
	} else if cfg.SocketPath != "" {
		opts = append(opts, client.WithHost("unix://"+cfg.SocketPath))
	}

	c, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Detect runtime: podman binary present?
	runtime := "docker"
	if _, err := exec.LookPath("podman"); err == nil {
		runtime = "podman"
	}

	return &DockerService{client: c, sshKey: cfg.SSHKey, sshHost: cfg.Host, runtime: runtime}, nil
}

// GetSSHHost returns the SSH host string (e.g., "user@host:port") for remote connections.
func (s *DockerService) GetSSHHost() string {
	if strings.HasPrefix(s.sshHost, "ssh://") {
		return strings.TrimPrefix(s.sshHost, "ssh://")
	}
	return ""
}

// GetSSHKey returns the SSH private key content.
func (s *DockerService) GetSSHKey() string {
	return s.sshKey
}

// GetDockerInfo returns the Docker server version string.
func (s *DockerService) GetDockerInfo(ctx context.Context) (string, error) {
	info, err := s.client.ServerVersion(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get Docker info: %w", err)
	}
	name := "Docker"
	if s.runtime == "podman" {
		name = "Podman"
	}
	return fmt.Sprintf("%s %s", name, info.Version), nil
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

	// Demultiplex Docker stdcopy stream into stdout
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, reader); err != nil {
		return nil, fmt.Errorf("failed to demultiplex logs: %w", err)
	}

	// Combine stdout + stderr, split by newlines
	combined := stdout.String() + stderr.String()
	logs := strings.Split(strings.TrimSpace(combined), "\n")
	return logs, nil
}

// GetStats returns container resource usage statistics including network traffic.
func (s *DockerService) GetStats(ctx context.Context, id string) (cpu float64, memory model.MemoryUsage, network model.NetworkTraffic, err error) {
	statsReader, err := s.client.ContainerStats(ctx, id, false)
	if err != nil {
		return 0, model.MemoryUsage{}, model.NetworkTraffic{}, fmt.Errorf("failed to get container stats: %w", err)
	}
	defer statsReader.Body.Close()

	var stats types.StatsJSON
	if err := json.NewDecoder(statsReader.Body).Decode(&stats); err != nil {
		return 0, model.MemoryUsage{}, model.NetworkTraffic{}, fmt.Errorf("failed to decode stats: %w", err)
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

	// Aggregate network traffic across all interfaces
	for _, net := range stats.Networks {
		network.RxBytes += net.RxBytes
		network.TxBytes += net.TxBytes
	}

	return cpu, memory, network, nil
}

// ListImages returns all Docker images with container counts.
func (s *DockerService) ListImages(ctx context.Context) ([]types.ImageSummary, error) {
	images, err := s.client.ImageList(ctx, types.ImageListOptions{})
	if err != nil {
		return nil, err
	}

	// Docker API returns Containers=-1 by default. Count from actual containers.
	containers, err := s.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return images, nil // return images even if container count fails
	}

	// Build image ID -> count map
	imageCounts := make(map[string]int64)
	for _, c := range containers {
		imageID := c.ImageID
		imageCounts[imageID]++
	}

	// Set counts on images (Docker defaults to -1, set to 0 if no containers)
	for i := range images {
		if count, ok := imageCounts[images[i].ID]; ok {
			images[i].Containers = count
		} else {
			images[i].Containers = 0
		}
	}

	return images, nil
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

// ListComposeProjects finds and returns Docker Compose projects from container labels.
// Discovery source depends on Docker connection:
//   - Remote (TCP): reads container labels from Docker API
//   - Local (socket): reads container labels first, then falls back to filesystem scan
func (s *DockerService) ListComposeProjects(ctx context.Context) ([]model.ComposeProject, error) {
	projects := make([]model.ComposeProject, 0)
	seen := make(map[string]bool)

	// 1. Discover compose projects from container labels
	containers, err := s.client.ContainerList(ctx, container.ListOptions{All: true})
	if err == nil {
		for _, c := range containers {
			projectName := c.Labels["com.docker.compose.project"]
			if projectName == "" || seen[projectName] {
				continue
			}
			seen[projectName] = true

			workingDir := c.Labels["com.docker.compose.project.working_dir"]

			project := model.ComposeProject{
				ID:   projectName,
				Name: projectName,
				Path: workingDir,
			}

			// Count running services
			for _, c2 := range containers {
				if c2.Labels["com.docker.compose.project"] == projectName {
					project.Services++
					if c2.State == "running" {
						project.Running++
					}
				}
			}
			if project.Running == project.Services {
				project.Status = "running"
			} else if project.Running > 0 {
				project.Status = "partial"
			} else {
				project.Status = "stopped"
			}

			projects = append(projects, project)
		}
	}

	return projects, nil
}

// ComposeUp runs docker-compose up.
// ComposeUp runs docker-compose up.
// CreateComposeProject creates a new compose project with the given name and content.
func (s *DockerService) CreateComposeProject(ctx context.Context, name, composeContent, envContent, basePath string) (*model.ComposeAction, error) {
	// Sanitize project name
	name = strings.TrimSpace(name)
	if name == "" {
		return &model.ComposeAction{Success: false, Message: "Project name is required"}, fmt.Errorf("empty name")
	}

	// Determine project directory
	projectDir := filepath.Join(basePath, name)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return &model.ComposeAction{Success: false, Message: "Failed to create project directory"}, err
	}

	// Write docker-compose.yml
	composePath := filepath.Join(projectDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		return &model.ComposeAction{Success: false, Message: "Failed to write compose file"}, err
	}

	// Write .env if provided
	if envContent != "" {
		envPath := filepath.Join(projectDir, ".env")
		if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
			return &model.ComposeAction{Success: false, Message: "Failed to write env file"}, err
		}
	}

	return &model.ComposeAction{
		Success: true,
		Message: fmt.Sprintf("Project %s created at %s", name, projectDir),
	}, nil
}

// GetComposeUpArgs detects container state and returns the appropriate compose command.
func (s *DockerService) GetComposeUpArgs(ctx context.Context, projectName string) ([]string, string) {
	containers, err := s.client.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+projectName)),
	})
	if err != nil || len(containers) == 0 {
		// No containers found — need full up, pull missing images
		return []string{"up", "-d", "--pull", "missing"}, "Compose up started"
	}

	running := 0
	for _, c := range containers {
		if c.State == "running" {
			running++
		}
	}

	if running == len(containers) {
		// All running — idempotent up
		return []string{"up", "-d"}, "Compose up started"
	}
	if running > 0 {
		// Partial — force recreate to sync
		return []string{"up", "-d", "--force-recreate"}, "Compose recreate started"
	}
	// All stopped — start them
	return []string{"start"}, "Compose start started"
}

func (s *DockerService) ComposeUp(ctx context.Context, projectPath string) (*model.ComposeAction, error) {
	cmd := composeCommand(ctx, s.runtime, "up", "-d")
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
	cmd := composeCommand(ctx, s.runtime, "down")
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

// ComposeClean stops and removes containers + volumes.
func (s *DockerService) ComposeClean(ctx context.Context, projectPath string) (*model.ComposeAction, error) {
	cmd := composeCommand(ctx, s.runtime, "down", "-v")
	cmd.Dir = projectPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return &model.ComposeAction{
			Success: false,
			Message: "Failed to clean compose project",
			Output:  string(output),
		}, err
	}

	return &model.ComposeAction{
		Success: true,
		Message: "Compose project cleaned",
		Output:  string(output),
	}, nil
}

// ComposeBuild runs docker-compose build.
func (s *DockerService) ComposeBuild(ctx context.Context, projectPath string) (*model.ComposeAction, error) {
	cmd := composeCommand(ctx, s.runtime, "build")
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
	cmd := composeCommand(ctx, s.runtime, "restart")
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
	cmd := composeCommand(ctx, s.runtime, "pull")
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

	cmd := composeCommand(ctx, s.runtime, "logs", "--tail", fmt.Sprintf("%d", tail))
	cmd.Dir = projectPath

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get compose logs: %w", err)
	}

	return strings.Split(string(output), "\n"), nil
}

// GetComposeFile reads a docker-compose file.
// GetComposeFile reads compose file content. Tries local first, then Docker API.
func (s *DockerService) GetComposeFile(ctx context.Context, projectPath string) (string, error) {
	// Try local first
	for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
		p := projectPath + "/" + name
		if content, err := readFileContent(p); err == nil {
			return content, nil
		}
	}

	// Find a running container from this project to read via Docker API
	containers, err := s.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return "", fmt.Errorf("failed to list containers: %w", err)
	}

	// Find project name from path
	for _, c := range containers {
		wd := c.Labels["com.docker.compose.project.working_dir"]
		if wd == projectPath && c.State == "running" {
			configFiles := c.Labels["com.docker.compose.project.config_files"]
			if configFiles != "" {
				firstFile := strings.Split(configFiles, ",")[0]
				firstFile = strings.TrimSpace(firstFile)
				files := map[string]*string{firstFile: new(string)}
				s.ReadComposeFilesViaDocker(ctx, files)
				if *files[firstFile] != "" {
					return *files[firstFile], nil
				}
			} else {
				for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
					path := projectPath + "/" + name
					files := map[string]*string{path: new(string)}
					s.ReadComposeFilesViaDocker(ctx, files)
					if *files[path] != "" {
						return *files[path], nil
					}
				}
			}
			break
		}
	}

	return "", fmt.Errorf("compose file not found at %s", projectPath)
}

// SaveComposeFile saves a docker-compose file.
func (s *DockerService) SaveComposeFile(projectPath string, content string) error {
	return writeFileContent(projectPath+"/docker-compose.yml", content)
}

// GetComposeEnv reads a .env file.
// GetComposeEnv reads .env file content. Tries local first, then Docker API.
func (s *DockerService) GetComposeEnv(ctx context.Context, projectPath string) (string, error) {
	// Try local first
	if content, err := readFileContent(projectPath + "/.env"); err == nil {
		return content, nil
	}

	// Find running container and read via Docker API
	containers, err := s.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return "", fmt.Errorf("failed to list containers: %w", err)
	}

	for _, c := range containers {
		wd := c.Labels["com.docker.compose.project.working_dir"]
		if wd == projectPath && c.State == "running" {
			path := projectPath + "/.env"
			files := map[string]*string{path: new(string)}
			s.ReadComposeFilesViaDocker(ctx, files)
			if *files[path] != "" {
				return *files[path], nil
			}
			break
		}
	}

	return "", fmt.Errorf(".env file not found at %s", projectPath)
}

// SaveComposeEnv saves a .env file.
func (s *DockerService) SaveComposeEnv(projectPath string, content string) error {
	return writeFileContent(projectPath+"/.env", content)
}

// DeleteComposeProject removes a compose project directory.
func (s *DockerService) DeleteComposeProject(projectPath string) error {
	return os.RemoveAll(projectPath)
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

// DetectShell probes the container for available shell interpreters.
func (s *DockerService) DetectShell(ctx context.Context, containerID string) string {
	candidates := []string{"/bin/bash", "/bin/zsh", "/bin/sh", "/bin/ash", "/usr/bin/bash", "/usr/bin/zsh", "/usr/bin/sh"}
	for _, shell := range candidates {
		execCfg := types.ExecConfig{
			AttachStdout: true,
			AttachStderr: true,
			Cmd:          []string{shell, "-c", "exit 0"},
		}
		resp, err := s.client.ContainerExecCreate(ctx, containerID, execCfg)
		if err != nil {
			continue
		}
		if err := s.client.ContainerExecStart(ctx, resp.ID, types.ExecStartCheck{}); err != nil {
			continue
		}
		inspect, err := s.client.ContainerExecInspect(ctx, resp.ID)
		if err != nil {
			continue
		}
		if inspect.ExitCode == 0 {
			return shell
		}
	}
	return "/bin/sh" // final fallback
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
	if s.runtime == "podman" {
		return s.pruneImagesCLI(ctx)
	}
	return s.client.ImagesPrune(ctx, filters.NewArgs(filters.Arg("dangling", "false")))
}

// pruneImagesCLI runs "podman image prune -a --force" to remove all unused images.
// Calculates space reclaimed by inspecting unused images before pruning.
func (s *DockerService) pruneImagesCLI(ctx context.Context) (types.ImagesPruneReport, error) {
	// Get images in use by containers
	containerCmd := exec.CommandContext(ctx, "podman", "ps", "-a", "--format", "{{.Image}}")
	containerOutput, _ := containerCmd.Output()
	inUse := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(containerOutput)), "\n") {
		if img := strings.TrimSpace(line); img != "" {
			inUse[img] = true
		}
	}
	// Get all image IDs and their repo:tag, sum sizes of unused ones
	repoCmd := exec.CommandContext(ctx, "podman", "images", "--format", "{{.ID}} {{.Repository}}:{{.Tag}}")
	repoOutput, _ := repoCmd.Output()
	var spaceReclaimed uint64
	for _, line := range strings.Split(strings.TrimSpace(string(repoOutput)), "\n") {
		parts := strings.Fields(strings.TrimSpace(line))
		if len(parts) == 2 && !inUse[parts[1]] {
			// Unused image — get size via inspect (returns bytes)
			sizeCmd := exec.CommandContext(ctx, "podman", "image", "inspect", "--format", "{{.Size}}", parts[0])
			if sizeOut, err := sizeCmd.Output(); err == nil {
				if size, err := strconv.ParseUint(strings.TrimSpace(string(sizeOut)), 10, 64); err == nil {
					spaceReclaimed += size
				}
			}
		}
	}
	// Now prune
	cmd := exec.CommandContext(ctx, "podman", "image", "prune", "-a", "--force")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return types.ImagesPruneReport{}, fmt.Errorf("podman image prune failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	var deleted []types.ImageDeleteResponseItem
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			deleted = append(deleted, types.ImageDeleteResponseItem{Deleted: line})
		}
	}
	return types.ImagesPruneReport{ImagesDeleted: deleted, SpaceReclaimed: spaceReclaimed}, nil
}

// KillContainer sends a signal to a container.
func (s *DockerService) KillContainer(ctx context.Context, id string, signal string) error {
	return s.client.ContainerKill(ctx, id, signal)
}

// ListNetworks returns all Docker networks with container counts.
func (s *DockerService) ListNetworks(ctx context.Context) ([]model.Network, error) {
	networks, err := s.client.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}

	// Count containers per network
	containers, err := s.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		containers = nil
	}

	networkCounts := make(map[string]int)
	for _, c := range containers {
		for _, net := range c.NetworkSettings.Networks {
			networkCounts[net.NetworkID]++
		}
	}

	result := make([]model.Network, 0, len(networks))
	for _, n := range networks {
		count := networkCounts[n.ID]
		result = append(result, model.Network{
			ID:         n.ID[:12],
			Name:       n.Name,
			Driver:     n.Driver,
			Scope:      n.Scope,
			Created:    n.Created.String(),
			Subnet:     getSubnet(n),
			Gateway:    getGateway(n),
			Internal:   n.Internal,
			Containers: count,
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
	if s.runtime == "podman" {
		return s.pruneNetworksCLI(ctx)
	}
	report, err := s.client.NetworksPrune(ctx, filters.NewArgs())
	if err != nil {
		return 0, fmt.Errorf("failed to prune networks: %w", err)
	}
	return int64(len(report.NetworksDeleted)), nil
}

// pruneNetworksCLI runs "podman network prune --force" to remove unused networks.
func (s *DockerService) pruneNetworksCLI(ctx context.Context) (int64, error) {
	cmd := exec.CommandContext(ctx, "podman", "network", "prune", "--force")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("podman network prune failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	// Count deleted networks (one ID per line)
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return int64(count), nil
}

// ReadComposeFilesViaDocker reads multiple compose files through a single temp container.
// Mounts the common parent directory once, reads all files via exec.
func (s *DockerService) ReadComposeFilesViaDocker(ctx context.Context, files map[string]*string) error {
	if len(files) == 0 {
		return nil
	}

	// Find the common parent directory of all files
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, filepath.Dir(p))
	}
	commonDir := findCommonParent(paths)

	// Create ONE temp container with the common dir mounted
	s.pullImageQuiet(ctx, "alpine:latest")
	containerName := fmt.Sprintf("boxbox-read-%d", time.Now().UnixNano())
	resp, err := s.client.ContainerCreate(ctx,
		&container.Config{
			Image: "alpine:latest",
			Cmd:   []string{"sleep", "300"}, // stay alive for 5 min
		},
		&container.HostConfig{
			Binds: []string{commonDir + ":/data:ro"},
		},
		nil, nil, containerName,
	)
	if err != nil {
		return fmt.Errorf("create read container: %w", err)
	}
	containerID := resp.ID
	defer func() {
		rmCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.client.ContainerRemove(rmCtx, containerID, container.RemoveOptions{Force: true})
	}()

	if err := s.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start read container: %w", err)
	}

	// Read each file via exec
	for filePath, dest := range files {
		// Compute relative path from commonDir
		relPath, err := filepath.Rel(commonDir, filePath)
		if err != nil {
			continue
		}
		containerPath := "/data/" + filepath.ToSlash(relPath)

		content, err := s.execInContainer(ctx, containerID, []string{"cat", containerPath})
		if err != nil {
			continue // file might not exist
		}
		*dest = content
	}

	return nil
}

// execInContainerRaw runs a command and returns raw stdout without trimming.
func (s *DockerService) execInContainerRaw(ctx context.Context, containerID string, cmd []string) (string, error) {
	execCfg := types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}
	resp, err := s.client.ContainerExecCreate(ctx, containerID, execCfg)
	if err != nil {
		return "", err
	}

	reader, err := s.client.ContainerExecAttach(ctx, resp.ID, types.ExecStartCheck{})
	if err != nil {
		return "", err
	}
	defer reader.Close()

	var stdout bytes.Buffer
	_, err = stdcopy.StdCopy(&stdout, io.Discard, reader.Reader)
	if err != nil {
		return "", err
	}
	return stdout.String(), nil
}

// execInContainer runs a command in a container and returns stdout
func (s *DockerService) execInContainer(ctx context.Context, containerID string, cmd []string) (string, error) {
	execCfg := types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}
	resp, err := s.client.ContainerExecCreate(ctx, containerID, execCfg)
	if err != nil {
		return "", err
	}

	reader, err := s.client.ContainerExecAttach(ctx, resp.ID, types.ExecStartCheck{})
	if err != nil {
		return "", err
	}
	defer reader.Close()

	// Demultiplex Docker stream
	var stdout bytes.Buffer
	_, err = stdcopy.StdCopy(&stdout, io.Discard, reader.Reader)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// pullImageQuiet pulls an image silently
func (s *DockerService) pullImageQuiet(ctx context.Context, imageRef string) {
	reader, err := s.client.ImagePull(ctx, imageRef, types.ImagePullOptions{})
	if err != nil {
		return
	}
	defer reader.Close()
	io.Copy(io.Discard, reader)
}

// stripDockerStreamHeaders removes 8-byte Docker multiplexed stream headers
func stripDockerStreamHeaders(data []byte) []byte {
	var result []byte
	for len(data) >= 8 {
		size := uint32(data[4])<<24 | uint32(data[5])<<16 | uint32(data[6])<<8 | uint32(data[7])
		if int(size) > len(data)-8 {
			break
		}
		result = append(result, data[8:8+size]...)
		data = data[8+size:]
	}
	return result
}

// findCommonParent finds the longest common prefix directory
func findCommonParent(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	common := filepath.Clean(paths[0])
	for _, p := range paths[1:] {
		cleaned := filepath.Clean(p)
		for !strings.HasPrefix(cleaned, common) {
			common = filepath.Dir(common)
			if common == "/" || common == "." {
				return "/"
			}
		}
	}
	return common
}



// GetHostIP returns the host machine's real LAN IP.
// Finds a running container and execs "cat /etc/hosts" to read the host IP.
func (s *DockerService) GetHostIP(ctx context.Context) string {
	containers, err := s.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil || len(containers) == 0 {
		return "localhost"
	}

	for _, c := range containers {
		if c.State != "running" {
			continue
		}
		output, err := s.execInContainer(ctx, c.ID, []string{"cat", "/proc/net/fib_trie"})
		if err != nil || output == "" {
			continue
		}
		// Parse /proc/net/fib_trie: find "192.168.x.x" host IP
		for _, line := range strings.Split(output, "\n") {
			for _, field := range strings.Fields(line) {
				ip := strings.Trim(field, "\n")
				// Strip CIDR notation
				if idx := strings.Index(ip, "/"); idx > 0 {
					ip = ip[:idx]
				}
				// Must be 192.168.x.x, not .0 (network) or .255 (broadcast)
				if strings.HasPrefix(ip, "192.168.") && !strings.HasSuffix(ip, ".0") && !strings.HasSuffix(ip, ".255") {
					return ip
				}
			}
		}
		break
	}
	return "localhost"
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

// ListFilesViaDocker lists files via SFTP.
func (s *DockerService) ListFilesViaDocker(ctx context.Context, hostPath string) ([]model.FileInfo, error) {
	c, err := s.newSFTPClient()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	entries, err := c.ReadDir(hostPath)
	if err != nil {
		return nil, fmt.Errorf("readdir %s: %w", hostPath, err)
	}

	var files []model.FileInfo
	for _, entry := range entries {
		info := entry
		files = append(files, model.FileInfo{
			Name:        info.Name(),
			Size:        info.Size(),
			IsDir:       info.IsDir(),
			ModTime:     info.ModTime(),
			Permissions: info.Mode().String(),
		})
	}
	return files, nil
}
// ReadFileViaDocker reads a file via SFTP.
func (s *DockerService) ReadFileViaDocker(ctx context.Context, hostPath string) ([]byte, error) {
	c, err := s.newSFTPClient()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	f, err := c.Open(hostPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", hostPath, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", hostPath, err)
	}
	return data, nil
}

// ReadFileRangeViaSFTP reads a byte range from a remote file via SFTP (for Range requests).
func (s *DockerService) ReadFileRangeViaSFTP(ctx context.Context, hostPath string, offset int64, length int64) ([]byte, error) {
	c, err := s.newSFTPClient()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	f, err := c.Open(hostPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", hostPath, err)
	}
	defer f.Close()

	buf := make([]byte, length)
	n, err := f.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("readat %s: %w", hostPath, err)
	}
	return buf[:n], nil
}

// GetFileSizeViaSFTP returns file size via SFTP.
func (s *DockerService) GetFileSizeViaSFTP(ctx context.Context, hostPath string) (int64, error) {
	c, err := s.newSFTPClient()
	if err != nil {
			return 0, err
	}
	defer c.Close()

	info, err := c.Stat(hostPath)
	if err != nil {
			return 0, err
	}
	return info.Size(), nil
}

// WriteFileViaSFTP uploads local data to a remote file via SFTP.
func (s *DockerService) WriteFileViaSFTP(ctx context.Context, hostPath string, data []byte) error {
	c, err := s.newSFTPClient()
	if err != nil {
		return err
	}
	defer c.Close()

	// Ensure parent directory exists
	parent := hostPath[:len(hostPath)-len(hostPath[strings.LastIndex(hostPath, "/"):])]
	if parent != "" {
		_ = c.MkdirAll(parent)
	}

	f, err := c.Create(hostPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", hostPath, err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", hostPath, err)
	}
	return nil
}

// RemoveViaSFTP deletes a file or directory via SFTP.
func (s *DockerService) RemoveViaSFTP(ctx context.Context, hostPath string) error {
	c, err := s.newSFTPClient()
	if err != nil {
		return err
	}
	defer c.Close()

	info, err := c.Stat(hostPath)
	if err != nil {
		return fmt.Errorf("stat %s: %w", hostPath, err)
	}
	if info.IsDir() {
		return removeDirSFTP(c, hostPath)
	}
	return c.Remove(hostPath)
}

func removeDirSFTP(c *sftp.Client, dir string) error {
	entries, err := c.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		entryPath := dir + "/" + entry.Name()
		if entry.IsDir() {
			if err := removeDirSFTP(c, entryPath); err != nil {
				return err
			}
		} else {
			if err := c.Remove(entryPath); err != nil {
				return err
			}
		}
	}
	return c.Remove(dir)
}

// RenameViaSFTP renames a file or directory via SFTP.
func (s *DockerService) RenameViaSFTP(ctx context.Context, oldPath, newPath string) error {
	c, err := s.newSFTPClient()
	if err != nil {
		return err
	}
	defer c.Close()
	return c.Rename(oldPath, newPath)
}

// MkdirViaSFTP creates a directory via SFTP.
func (s *DockerService) MkdirViaSFTP(ctx context.Context, hostPath string) error {
	c, err := s.newSFTPClient()
	if err != nil {
		return err
	}
	defer c.Close()
	return c.MkdirAll(hostPath)
}

// GetFileInfoViaDocker gets file info via SFTP.
func (s *DockerService) GetFileInfoViaDocker(ctx context.Context, hostPath string) (*model.FileInfo, error) {
	c, err := s.newSFTPClient()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	info, err := c.Stat(hostPath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", hostPath, err)
	}

	return &model.FileInfo{
		Name:        info.Name(),
		Path:        hostPath,
		Size:        info.Size(),
		IsDir:       info.IsDir(),
		ModTime:     info.ModTime(),
		Permissions: info.Mode().String(),
	}, nil
}
// parseLsOutput parses `ls -la` output into FileInfo slice.
func parseLsOutput(output string) []model.FileInfo {
	var files []model.FileInfo
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "total") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}
		name := strings.Join(fields[8:], " ")
		if name == "." || name == ".." {
			continue
		}
		isDir := fields[0][0] == 'd'
		size, _ := strconv.ParseInt(fields[4], 10, 64)
		// fields[5] = month, fields[6] = day, fields[7] = time/year
		modStr := fields[5] + " " + fields[6] + " " + fields[7]
		modTime, _ := time.Parse("Jan 2 15:04", modStr)
		if modTime.Year() == 0 {
			modTime, _ = time.Parse("Jan 2 2006", modStr)
		}

		files = append(files, model.FileInfo{
			Name:    name,
			Size:    size,
			IsDir:   isDir,
			ModTime: modTime,
		})
	}
	return files
}

// Runtime returns the detected container runtime ("docker" or "podman").
func (s *DockerService) Runtime() string {
	return s.runtime
}
