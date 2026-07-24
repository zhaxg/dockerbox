// Package model contains Docker-related data models.
package model

import "time"

// Container represents a Docker container with its status and resource usage.
type Container struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Image     string            `json:"image"`
	Status    string            `json:"status"` // running, stopped, paused, created
	State     string            `json:"state"`  // running, exited, paused, created
	Created   time.Time         `json:"created"`
	Ports     []PortBinding     `json:"ports"`
	CPU       float64           `json:"cpu"`    // CPU usage percentage
	Memory    MemoryUsage       `json:"memory"`
	Network   NetworkTraffic    `json:"network"`
	Labels    map[string]string `json:"labels"`
}

// NetworkTraffic represents network I/O stats.
type NetworkTraffic struct {
	RxBytes uint64 `json:"rxBytes"`
	TxBytes uint64 `json:"txBytes"`
}

// PortBinding represents a port mapping between host and container.
type PortBinding struct {
	HostIP        string `json:"hostIp"`
	HostPort      string `json:"hostPort"`
	ContainerPort string `json:"containerPort"`
	Protocol      string `json:"protocol"` // tcp, udp
}

// MemoryUsage represents memory usage statistics.
type MemoryUsage struct {
	Usage   int64   `json:"usage"`   // bytes
	Limit   int64   `json:"limit"`   // bytes
	Percent float64 `json:"percent"` // percentage
}

// ComposeProject represents a Docker Compose project.
type ComposeProject struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Status   string `json:"status"`   // running, stopped, partial, error
	Services int    `json:"services"` // total services
	Running  int    `json:"running"`  // running services
	File     string `json:"file"`     // docker-compose.yml content
	EnvFile  string `json:"envFile"`  // .env content
}

// DockerStats represents system-wide Docker statistics.
type DockerStats struct {
	Containers struct {
		Total   int `json:"total"`
		Running int `json:"running"`
		Stopped int `json:"stopped"`
		Paused  int `json:"paused"`
	} `json:"containers"`
	Compose struct {
		Total   int `json:"total"`
		Running int `json:"running"`
	} `json:"compose"`
	Images struct {
		Total int `json:"total"`
		Size  int64 `json:"size"` // bytes
	} `json:"images"`
}

// ContainerLogs represents container log output.
type ContainerLogs struct {
	Lines []string `json:"lines"`
}

// ComposeAction represents a Compose operation result.
type ComposeAction struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Output  string `json:"output,omitempty"`
}

// Network represents a Docker network.
type Network struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Scope      string `json:"scope"`
	Created    string `json:"created"`
	Subnet     string `json:"subnet"`
	Gateway    string `json:"gateway"`
	Internal   bool   `json:"internal"`
	Containers int    `json:"containers"`
}
