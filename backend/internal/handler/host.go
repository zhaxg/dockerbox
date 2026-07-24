package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jR4dh3y/BoxBox/backend/internal/config"
	"github.com/jR4dh3y/BoxBox/backend/internal/model"
	"github.com/jR4dh3y/BoxBox/backend/internal/service"
)

// HostHandler manages Docker host CRUD and connection testing.
type HostHandler struct {
	getConfig func() *model.ServerConfig
	saveHosts func(hosts *model.DockerHostsConfig) error
}

// NewHostHandler creates a new host handler.
func NewHostHandler(getConfig func() *model.ServerConfig, saveHosts func(hosts *model.DockerHostsConfig) error) *HostHandler {
	return &HostHandler{getConfig: getConfig, saveHosts: saveHosts}
}

// RegisterRoutes registers host routes.
func (h *HostHandler) RegisterRoutes(r chi.Router) {
	r.Get("/", h.ListHosts)
	r.Post("/", h.CreateHost)
	r.Put("/{id}", h.UpdateHost)
	r.Delete("/{id}", h.DeleteHost)
	r.Post("/{id}/test", h.TestConnection)
	r.Get("/{id}/stats", h.GetHostStats)
	r.Post("/sshkey", h.SSHKeyGen)
	r.Post("/pushkey", h.SSHPushKey)
}

func (h *HostHandler) ensureDefault(hosts *model.DockerHostsConfig) {
	if hosts.Hosts == nil {
		hosts.Hosts = make(map[string]*model.DockerHost)
	}
	if hosts.Default == "" {
		for id := range hosts.Hosts {
			hosts.Default = id
			return
		}
	}
}

// ListHosts returns all configured Docker hosts.
func (h *HostHandler) ListHosts(w http.ResponseWriter, r *http.Request) {
	cfg := h.getConfig()
	if cfg.DockerHosts == nil {
		cfg.DockerHosts = &model.DockerHostsConfig{Hosts: make(map[string]*model.DockerHost)}
	}
	writeJSON(w, cfg.DockerHosts, http.StatusOK)
}

// CreateHost adds a new Docker host.
func (h *HostHandler) CreateHost(w http.ResponseWriter, r *http.Request) {
	var host model.DockerHost
	if err := json.NewDecoder(r.Body).Decode(&host); err != nil {
		writeError(w, "Invalid request body", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if host.ID == "" || host.Name == "" || host.Driver == "" || host.Endpoint == "" {
		writeError(w, "id, name, driver, endpoint are required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}
	if host.Driver != "tcp" && host.Driver != "ssh" && host.Driver != "socket" {
		writeError(w, "driver must be tcp, ssh, or socket", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	cfg := h.getConfig()
	if cfg.DockerHosts == nil {
		cfg.DockerHosts = &model.DockerHostsConfig{Hosts: make(map[string]*model.DockerHost)}
	}
	if _, exists := cfg.DockerHosts.Hosts[host.ID]; exists {
		writeError(w, "Host ID already exists", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if host.MountPoints == nil {
		host.MountPoints = make(map[string]*model.HostMountPoint)
	}

	cfg.DockerHosts.Hosts[host.ID] = &host
	h.ensureDefault(cfg.DockerHosts)

	if err := h.saveHosts(cfg.DockerHosts); err != nil {
		log.Printf("saveHosts error: %v", err)
		writeError(w, "Failed to save: "+err.Error(), model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, host, http.StatusCreated)
}

// UpdateHost updates an existing Docker host.
func (h *HostHandler) UpdateHost(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var updates model.DockerHost
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, "Invalid request body", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	cfg := h.getConfig()
	if cfg.DockerHosts == nil || cfg.DockerHosts.Hosts == nil {
		writeError(w, "Host not found", model.ErrCodeValidationError, http.StatusNotFound)
		return
	}

	existing, ok := cfg.DockerHosts.Hosts[id]
	if !ok {
		writeError(w, "Host not found", model.ErrCodeValidationError, http.StatusNotFound)
		return
	}

	if updates.Name != "" {
		existing.Name = updates.Name
	}
	if updates.Driver != "" {
		existing.Driver = updates.Driver
	}
	if updates.Endpoint != "" {
		existing.Endpoint = updates.Endpoint
	}
	existing.SSHKey = updates.SSHKey
	existing.SSHPubKey = updates.SSHPubKey
	existing.Tags = updates.Tags
	if updates.MountPoints != nil {
		existing.MountPoints = updates.MountPoints
	}

	if err := h.saveHosts(cfg.DockerHosts); err != nil {
		writeError(w, "Failed to save", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, existing, http.StatusOK)
}

// DeleteHost removes a Docker host.
func (h *HostHandler) DeleteHost(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cfg := h.getConfig()

	if cfg.DockerHosts == nil || cfg.DockerHosts.Hosts == nil {
		writeError(w, "Host not found", model.ErrCodeValidationError, http.StatusNotFound)
		return
	}

	if _, ok := cfg.DockerHosts.Hosts[id]; !ok {
		writeError(w, "Host not found", model.ErrCodeValidationError, http.StatusNotFound)
		return
	}

	delete(cfg.DockerHosts.Hosts, id)
	if cfg.DockerHosts.Default == id {
		cfg.DockerHosts.Default = ""
		h.ensureDefault(cfg.DockerHosts)
	}

	if err := h.saveHosts(cfg.DockerHosts); err != nil {
		writeError(w, "Failed to save", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Host deleted"}, http.StatusOK)
}

func buildDockerCfg(target *model.DockerHost) service.DockerServiceConfig {
	cfg := service.DockerServiceConfig{}
	switch target.Driver {
	case "tcp":
		cfg.Host = "tcp://" + target.Endpoint
	case "socket":
		if strings.HasPrefix(target.Endpoint, "/") {
			cfg.SocketPath = target.Endpoint
		} else {
			cfg.SocketPath = "/var/run/docker.sock"
		}
	case "ssh":
		cfg.Host = "ssh://" + target.Endpoint
		if target.SSHKey != "" {
			cfg.SSHKey = target.SSHKey
		}
	}
	return cfg
}

// GetHostStats returns connection status and container counts.
func (h *HostHandler) GetHostStats(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cfg := h.getConfig()
	if cfg.DockerHosts == nil || cfg.DockerHosts.Hosts == nil {
		writeError(w, "Host not found", model.ErrCodeValidationError, http.StatusNotFound)
		return
	}
	target, ok := cfg.DockerHosts.Hosts[id]
	if !ok {
		writeError(w, "Host not found", model.ErrCodeValidationError, http.StatusNotFound)
		return
	}

	result := map[string]interface{}{"status": "offline", "total": 0, "running": 0, "stopped": 0}
	dockerCfg := buildDockerCfg(target)
	dockerSvc, err := service.NewDockerService(dockerCfg)
	if err != nil {
		result["message"] = err.Error()
		writeJSON(w, result, http.StatusOK)
		return
	}
	if _, err := dockerSvc.GetDockerInfo(r.Context()); err != nil {
		result["message"] = err.Error()
		writeJSON(w, result, http.StatusOK)
		return
	}
	result["status"] = "online"
	containers, err := dockerSvc.ListContainers(r.Context())
	if err == nil {
		result["total"] = len(containers)
		running := 0
		for _, c := range containers {
			if c.State == "running" { running++ }
		}
		result["running"] = running
		result["stopped"] = len(containers) - running
	}
	writeJSON(w, result, http.StatusOK)
}

// TestConnection tests connectivity to a Docker host.
func (h *HostHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cfg := h.getConfig()

	if cfg.DockerHosts == nil || cfg.DockerHosts.Hosts == nil {
		writeError(w, "Host not found", model.ErrCodeValidationError, http.StatusNotFound)
		return
	}

	target, ok := cfg.DockerHosts.Hosts[id]
	if !ok {
		writeError(w, "Host not found", model.ErrCodeValidationError, http.StatusNotFound)
		return
	}

	dockerCfg := buildDockerCfg(target)

	dockerSvc, err := service.NewDockerService(dockerCfg)
	if err != nil {
		writeJSON(w, map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("Failed to create client: %v", err),
		}, http.StatusOK)
		return
	}

	info, err := dockerSvc.GetDockerInfo(r.Context())
	if err != nil {
		writeJSON(w, map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("Connection failed: %v", err),
		}, http.StatusOK)
		return
	}

	writeJSON(w, map[string]interface{}{
		"status":  "ok",
		"message": fmt.Sprintf("Connected: Docker %s", info),
	}, http.StatusOK)
}

// SSHKeyPairInstructions returns instructions for SSH key setup.
func (h *HostHandler) SSHKeyPairInstructions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"generate_key":    "ssh-keygen -t ed25519 -C \"boxbox@$(hostname)\" -f ~/.ssh/boxbox_ed25519 -N \"\"",
		"copy_key":        "ssh-copy-id -i ~/.ssh/boxbox_ed25519.pub USER@HOST",
		"test_connect":    "ssh -i ~/.ssh/boxbox_ed25519 USER@HOST docker info",
		"public_key_path": "~/.ssh/boxbox_ed25519.pub",
	}, http.StatusOK)
}

// SSHKeyGen generates an ED25519 key pair and returns them.
func (h *HostHandler) SSHKeyGen(w http.ResponseWriter, r *http.Request) {
	// Generate ED25519 key pair via temp files
	tmpDir, err := os.MkdirTemp("", "boxbox-keygen-*")
	if err != nil {
		writeJSON(w, map[string]interface{}{"status": "error", "message": fmt.Sprintf("failed to create temp dir: %v", err)}, http.StatusOK)
		return
	}
	defer os.RemoveAll(tmpDir)

	keyPath := tmpDir + "/id_ed25519"
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-C", "boxbox", "-f", keyPath, "-N", "")
	if err := cmd.Run(); err != nil {
		writeJSON(w, map[string]interface{}{"status": "error", "message": fmt.Sprintf("keygen failed: %v", err)}, http.StatusOK)
		return
	}

	privKey, err := os.ReadFile(keyPath)
	if err != nil {
		writeJSON(w, map[string]interface{}{"status": "error", "message": fmt.Sprintf("failed to read private key: %v", err)}, http.StatusOK)
		return
	}

	pubKey, err := os.ReadFile(keyPath + ".pub")
	if err != nil {
		writeJSON(w, map[string]interface{}{"status": "error", "message": fmt.Sprintf("failed to read public key: %v", err)}, http.StatusOK)
		return
	}

	writeJSON(w, map[string]interface{}{
		"private_key": strings.TrimSpace(string(privKey)),
		"public_key":  strings.TrimSpace(string(pubKey)),
	}, http.StatusOK)
}

// SSHPushKey pushes the public key to a remote server via ssh-copy-id.
func (h *HostHandler) SSHPushKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		User     string `json:"user"`
		Host     string `json:"host"`
		Port     string `json:"port"`
		Password string `json:"password"`
		PubKey   string `json:"pubkey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if req.User == "" || req.Host == "" || req.Password == "" || req.PubKey == "" {
		writeError(w, "user, host, password, pubkey required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if req.Port == "" {
		req.Port = "22"
	}

	// Check sshpass
	if exec.Command("which", "sshpass").Run() != nil {
		writeJSON(w, map[string]interface{}{
			"status":  "error",
			"message": "sshpass not installed. Install: apt install sshpass",
		}, http.StatusOK)
		return
	}

	// Push pubkey via ssh - pipe via stdin to avoid shell escaping
	cmd := exec.Command("sshpass", "-p", req.Password, "ssh",
		"-o", "StrictHostKeyChecking=no",
		"-p", req.Port,
		req.User+"@"+req.Host,
		"sh", "-c", "mkdir -p ~/.ssh && chmod 700 ~/.ssh && cat >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys")
	cmd.Stdin = strings.NewReader(req.PubKey + "\n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		writeJSON(w, map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("push failed: %s", string(output)),
		}, http.StatusOK)
		return
	}

	writeJSON(w, map[string]interface{}{
		"status":  "ok",
		"message": "公钥推送成功",
	}, http.StatusOK)
}

// saveHostsToFile persists hosts to config.yaml.
func saveHostsToFile(hosts *model.DockerHostsConfig) error {
	cfg, err := config.LoadWithReport("")
	if err != nil {
		return err
	}
	cfg.Config.DockerHosts = hosts
	return config.Save(cfg.Config)
}
