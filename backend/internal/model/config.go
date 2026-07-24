package model

import (
	"fmt"
	"strings"
)

// MountPoint represents a configured filesystem location accessible through the file manager
type MountPoint struct {
	Name         string `json:"name" mapstructure:"name"`
	Path         string `json:"path" mapstructure:"path"`
	ReadOnly     bool   `json:"readOnly" mapstructure:"read_only"`
	AutoDiscover bool   `json:"autoDiscover" mapstructure:"auto_discover"`
}

// DockerHostsConfig holds all Docker host configurations.
type DockerHostsConfig struct {
	Default string                    `json:"default" mapstructure:"default"` // ID of the default host
	Hosts   map[string]*DockerHost    `json:"hosts" mapstructure:"hosts"`
}

// DockerHost represents a remote or local Docker host connection.
type DockerHost struct {
	ID          string              `json:"id" mapstructure:"id"`
	Name        string              `json:"name" mapstructure:"name"`
	Driver      string              `json:"driver" mapstructure:"driver"`       // tcp | ssh | socket
	Endpoint    string              `json:"endpoint" mapstructure:"endpoint"`   // e.g. 192.168.1.100:2375, user@192.168.1.100, /var/run/docker.sock
	SSHKey      string              `json:"sshKey,omitempty" mapstructure:"ssh_key"`
	SSHPubKey   string              `json:"sshPubKey,omitempty" mapstructure:"ssh_pub_key"`
	Tags        []string            `json:"tags,omitempty" mapstructure:"tags"`
	MountPoints map[string]*HostMountPoint `json:"mountPoints" mapstructure:"mount_points"`
}

// HostMountPoint represents a browsable directory on a Docker host.
type HostMountPoint struct {
	Path     string `json:"path" yaml:"path" mapstructure:"path"`
	ReadOnly bool   `json:"readOnly" yaml:"read_only" mapstructure:"read_only"`
	IsDocker bool   `json:"isDocker" yaml:"is_docker" mapstructure:"is_docker"`
}

// ServerConfig contains all server configuration options
type ServerConfig struct {
	Port        int          `mapstructure:"port"`
	Host        string       `mapstructure:"host"`
	MountPoints []MountPoint `mapstructure:"mount_points"`
	JWTSecret   string       `mapstructure:"jwt_secret"`
	MaxUploadMB int          `mapstructure:"max_upload_mb"`
	ChunkSizeMB int          `mapstructure:"chunk_size_mb"`
	DataDir     string       `mapstructure:"data_dir"`

	// Docker settings
	DockerHost    string   `mapstructure:"docker_host"`     // e.g. tcp://192.168.1.100:2375
	DockerSocket  string   `mapstructure:"docker_socket"`   // e.g. /var/run/docker.sock
	ComposePaths  []string     `mapstructure:"compose_paths"`   // directories to scan for compose projects
	DockerHosts   *DockerHostsConfig `mapstructure:"dockerhosts"`

	// Security settings
	Users          map[string]string `mapstructure:"users"`           // username -> password
	AllowedOrigins []string          `mapstructure:"allowed_origins"` // WebSocket/CORS allowed origins
	RateLimitRPS   float64           `mapstructure:"rate_limit_rps"`  // Auth endpoint rate limit (requests per second)
}

// Validate checks that the configuration is valid
func (c *ServerConfig) Validate() error {
	if c.JWTSecret == "" {
		return fmt.Errorf("jwt_secret is required")
	}
	if strings.Contains(strings.ToLower(c.JWTSecret), "change-me") {
		return fmt.Errorf("jwt_secret must be changed from the placeholder value")
	}

	if len(c.Users) == 0 {
		return fmt.Errorf("at least one user is required")
	}
	for username, password := range c.Users {
		if username == "" {
			return fmt.Errorf("usernames cannot be empty")
		}
		if password == "" {
			return fmt.Errorf("password for user %q is required", username)
		}
		if strings.Contains(strings.ToLower(password), "change-me") {
			return fmt.Errorf("password for user %q must be changed from the placeholder value", username)
		}
	}

	if len(c.MountPoints) == 0 {
		return fmt.Errorf("at least one mount_point is required")
	}

	for i, mp := range c.MountPoints {
		if mp.Name == "" {
			return fmt.Errorf("mount_point[%d].name is required", i)
		}
		if mp.Path == "" {
			return fmt.Errorf("mount_point[%d].path is required", i)
		}
	}

	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}

	if c.MaxUploadMB < 1 {
		return fmt.Errorf("max_upload_mb must be at least 1")
	}

	if c.ChunkSizeMB < 1 {
		return fmt.Errorf("chunk_size_mb must be at least 1")
	}

	return nil
}
