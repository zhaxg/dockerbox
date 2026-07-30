package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/ssh"
	"dockerbox/backend/internal/config"
	"dockerbox/backend/internal/model"
	"dockerbox/backend/internal/service"
)

// HostHandler manages Docker host CRUD and connection testing.
type HostHandler struct {
	getConfig             func() *model.ServerConfig
	saveHosts             func(hosts *model.DockerHostsConfig) error
	onMountPointsUpdate   func(hostID string, mps []model.MountPoint)
}

// NewHostHandler creates a new host handler.
func NewHostHandler(getConfig func() *model.ServerConfig, saveHosts func(hosts *model.DockerHostsConfig) error, onMountPointsUpdate func(hostID string, mps []model.MountPoint)) *HostHandler {
	return &HostHandler{getConfig: getConfig, saveHosts: saveHosts, onMountPointsUpdate: onMountPointsUpdate}
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
func generateHostID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	id := make([]byte, 6)
	for i := range id {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		id[i] = chars[n.Int64()]
	}
	return "host" + string(id)
}

func (h *HostHandler) CreateHost(w http.ResponseWriter, r *http.Request) {
	var host model.DockerHost
	if err := json.NewDecoder(r.Body).Decode(&host); err != nil {
		writeError(w, "Invalid request body", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if host.DisplayName == "" || host.Driver == "" || host.Endpoint == "" {
		writeError(w, "name, driver, endpoint are required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}
	if host.Driver != "tcp" && host.Driver != "ssh" && host.Driver != "socket" {
		writeError(w, "driver must be tcp, ssh, or socket", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	// Validate SSH configuration
	if host.Driver == "ssh" {
		if err := validateSSHConfig(host.Endpoint, host.SSHKey); err != nil {
			writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
			return
		}
		// Validate mount point paths
		if err := validateMountPoints(host.Endpoint, host.SSHKey, host.MountPoints); err != nil {
			writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
			return
		}
	}

	cfg := h.getConfig()
	if cfg.DockerHosts == nil {
		cfg.DockerHosts = &model.DockerHostsConfig{Hosts: make(map[string]*model.DockerHost)}
	}

	// Check for duplicate name or endpoint
	for _, existing := range cfg.DockerHosts.Hosts {
		if existing.DisplayName == host.DisplayName {
			writeError(w, "A host with this name already exists", model.ErrCodeValidationError, http.StatusConflict)
			return
		}
		if existing.Endpoint == host.Endpoint {
			writeError(w, "A host with this endpoint already exists", model.ErrCodeValidationError, http.StatusConflict)
			return
		}
	}

	hostID := generateHostID()
	if _, exists := cfg.DockerHosts.Hosts[hostID]; exists {
		writeError(w, "Host ID already exists", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if host.MountPoints == nil {
		host.MountPoints = make(map[string]*model.HostMountPoint)
	}

	cfg.DockerHosts.Hosts[hostID] = &host
	h.ensureDefault(cfg.DockerHosts)

	if err := h.saveHosts(cfg.DockerHosts); err != nil {
		log.Printf("saveHosts error: %v", err)
		writeError(w, "Failed to save: "+err.Error(), model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}
	ensureSSHKey(hostID, host.SSHKey, host.Endpoint)

	// Return with ID set for frontend
	writeJSON(w, map[string]interface{}{"id": hostID, "host": host}, http.StatusCreated)
}

// UpdateHost updates an existing Docker host.
func (h *HostHandler) UpdateHost(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var updates struct {
		model.DockerHost
		IsDefault bool `json:"isDefault"`
	}
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

	// Check for duplicate name or endpoint (excluding self)
	if updates.DisplayName != "" || updates.Endpoint != "" {
		for otherID, other := range cfg.DockerHosts.Hosts {
			if otherID == id {
				continue
			}
			if updates.DisplayName != "" && other.DisplayName == updates.DisplayName {
				writeError(w, "A host with this name already exists", model.ErrCodeValidationError, http.StatusConflict)
				return
			}
			if updates.Endpoint != "" && other.Endpoint == updates.Endpoint {
				writeError(w, "A host with this endpoint already exists", model.ErrCodeValidationError, http.StatusConflict)
				return
			}
		}
	}

	// Validate SSH configuration if driver is ssh
	if existing.Driver == "ssh" {
		endpoint := updates.Endpoint
		if endpoint == "" {
			endpoint = existing.Endpoint
		}
		sshKey := updates.SSHKey
		if sshKey == "" {
			sshKey = existing.SSHKey
		}
		if err := validateSSHConfig(endpoint, sshKey); err != nil {
			writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
			return
		}
		// Validate mount point paths
		mountPoints := updates.MountPoints
		if mountPoints == nil {
			mountPoints = existing.MountPoints
		}
		if err := validateMountPoints(endpoint, sshKey, mountPoints); err != nil {
			writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
			return
		}
	}

	if updates.DisplayName != "" {
		existing.DisplayName = updates.DisplayName
	}
	if updates.Driver != "" {
		existing.Driver = updates.Driver
	}
	if updates.Endpoint != "" {
		existing.Endpoint = updates.Endpoint
	}
	if updates.SSHKey != "" {
		existing.SSHKey = updates.SSHKey
	}
	if updates.SSHPubKey != nil && *updates.SSHPubKey != "" {
		existing.SSHPubKey = updates.SSHPubKey
	}
	existing.Tags = updates.Tags

	// Update default host
	if updates.IsDefault {
		cfg.DockerHosts.Default = id
	} else if cfg.DockerHosts.Default == id {
		cfg.DockerHosts.Default = ""
	}
	if updates.MountPoints != nil {
		existing.MountPoints = updates.MountPoints
	}

	if err := h.saveHosts(cfg.DockerHosts); err != nil {
		writeError(w, "Failed to save", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}
	ensureSSHKey(id, existing.SSHKey, existing.Endpoint)

	// Update file handler mount points
	if h.onMountPointsUpdate != nil && updates.MountPoints != nil {
		var mps []model.MountPoint
		for name, mp := range existing.MountPoints {
			mps = append(mps, model.MountPoint{
				Name:     name,
				Path:     mp.Path,
				ReadOnly: mp.ReadOnly,
			})
		}
		h.onMountPointsUpdate(id, mps)
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

func buildDockerCfg(hostID string, target *model.DockerHost) service.DockerServiceConfig {
	cfg := service.DockerServiceConfig{HostID: hostID}
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
	dockerCfg := buildDockerCfg(id, target)
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

	dockerCfg := buildDockerCfg(id, target)

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
		"message": fmt.Sprintf("Connected: %s", info),
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

// SSHKeyGen generates an ED25519 key pair using Go's native crypto/ed25519.
func (h *HostHandler) SSHKeyGen(w http.ResponseWriter, r *http.Request) {
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		writeJSON(w, map[string]interface{}{"status": "error", "message": fmt.Sprintf("keygen failed: %v", err)}, http.StatusOK)
		return
	}

	// Encode private key in PEM format
	privPEM, err := ssh.MarshalPrivateKey(privKey, "boxbox")
	if err != nil {
		writeJSON(w, map[string]interface{}{"status": "error", "message": fmt.Sprintf("private key encoding failed: %v", err)}, http.StatusOK)
		return
	}
	privBytes := pem.EncodeToMemory(privPEM)

	// Encode public key in OpenSSH format
	pub, ok := privKey.Public().(ed25519.PublicKey)
	if !ok {
		writeJSON(w, map[string]interface{}{"status": "error", "message": "failed to get public key"}, http.StatusOK)
		return
	}
	pubKey, err := ssh.NewPublicKey(pub)
	if err != nil {
		writeJSON(w, map[string]interface{}{"status": "error", "message": fmt.Sprintf("public key encoding failed: %v", err)}, http.StatusOK)
		return
	}
	pubBytes := ssh.MarshalAuthorizedKey(pubKey)

	writeJSON(w, map[string]interface{}{
		"private_key": strings.TrimSpace(string(privBytes)),
		"public_key":  strings.TrimSpace(string(pubBytes)),
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

	// Push pubkey via Go native SSH with password authentication
	config := &ssh.ClientConfig{
		User:            req.User,
		Auth:            []ssh.AuthMethod{ssh.Password(req.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	conn, err := ssh.Dial("tcp", req.Host+":"+req.Port, config)
	if err != nil {
		writeJSON(w, map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("SSH connection failed: %v", err),
		}, http.StatusOK)
		return
	}
	defer conn.Close()

	session, err := conn.NewSession()
	if err != nil {
		writeJSON(w, map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("SSH session failed: %v", err),
		}, http.StatusOK)
		return
	}
	defer session.Close()

	session.Stdin = strings.NewReader(req.PubKey + "\n")
	if err := session.Run("sh -c 'mkdir -p ~/.ssh && chmod 700 ~/.ssh && cat >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys'"); err != nil {
		writeJSON(w, map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("push failed: %v", err),
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

// ensureSSHKey writes the private key and updates ~/.ssh/config for Docker SDK connhelper.
func ensureSSHKey(hostID, sshKey, endpoint string) {
	if sshKey == "" {
		return
	}
	homeDir, _ := os.UserHomeDir()
	sshDir := filepath.Join(homeDir, ".ssh")
	os.MkdirAll(sshDir, 0700)

	// Write private key
	keyPath := filepath.Join(sshDir, "boxbox_"+hostID)
	os.WriteFile(keyPath, []byte(sshKey+"\n"), 0600)

	// Parse endpoint to get user@host:port
	user, hostPort := "root", endpoint
	if idx := strings.Index(endpoint, "@"); idx >= 0 {
		user = endpoint[:idx]
		hostPort = endpoint[idx+1:]
	}
	host, port := hostPort, "22"
	if idx := strings.LastIndex(hostPort, ":"); idx >= 0 {
		host = hostPort[:idx]
		port = hostPort[idx+1:]
	}

	// Update ~/.ssh/config - remove ALL old entries for this host first
	configPath := filepath.Join(sshDir, "config")
	configData, _ := os.ReadFile(configPath)
	configStr := string(configData)

	marker := "# boxbox:" + hostID
	lines := strings.Split(configStr, "\n")
	var newLines []string
	skip := false
	for _, line := range lines {
		if strings.Contains(line, marker) {
			skip = true
			continue
		}
		if skip {
			if strings.HasPrefix(strings.TrimSpace(line), "Host ") || strings.TrimSpace(line) == "" {
				skip = false
			} else {
				continue
			}
		}
		newLines = append(newLines, line)
	}

	// Add new entry
	entry := fmt.Sprintf("\nHost %s %s\n  HostName %s\n  Port %s\n  User %s\n  IdentityFile %s\n  StrictHostKeyChecking no\n  # boxbox:%s",
		host, hostID, host, port, user, keyPath, hostID)
	newLines = append(newLines, entry)

	os.WriteFile(configPath, []byte(strings.Join(newLines, "\n")), 0600)
}

// validateSSHConfig validates SSH endpoint format and key.
func validateSSHConfig(endpoint, sshKey string) error {
	// Validate endpoint format: should be user@host or user@host:port
	if !strings.Contains(endpoint, "@") {
		return fmt.Errorf("SSH endpoint must be in user@host:port format (e.g., root@192.168.1.100:22)")
	}

	parts := strings.SplitN(endpoint, "@", 2)
	user := parts[0]
	hostPort := parts[1]

	if user == "" {
		return fmt.Errorf("SSH user is required in endpoint")
	}

	// Parse host:port
	host := hostPort
	port := "22"
	if idx := strings.LastIndex(hostPort, ":"); idx > 0 {
		host = hostPort[:idx]
		port = hostPort[idx+1:]
		// Validate port is numeric
		if _, err := fmt.Sscanf(port, "%d"); err != nil {
			return fmt.Errorf("SSH port must be a number")
		}
	}

	if host == "" {
		return fmt.Errorf("SSH host is required in endpoint")
	}

	// Validate SSH key is provided
	if sshKey == "" {
		return fmt.Errorf("SSH private key is required")
	}

	// Validate key format
	trimmedKey := strings.TrimSpace(sshKey)
	if !strings.Contains(trimmedKey, "PRIVATE KEY") {
		return fmt.Errorf("SSH key must be a valid private key (PEM format)")
	}

	return nil
}

// validateMountPoints validates that mount point paths exist on the remote host.
func validateMountPoints(endpoint, sshKey string, mountPoints map[string]*model.HostMountPoint) error {
	if mountPoints == nil || sshKey == "" {
		return nil
	}

	// Connect via SSH and check paths
	config := &ssh.ClientConfig{
		User:            strings.SplitN(endpoint, "@", 2)[0],
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(parseSSHKey(sshKey))},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	// Parse host:port
	hostPort := strings.SplitN(endpoint, "@", 2)[1]
	host := hostPort
	port := "22"
	if idx := strings.LastIndex(hostPort, ":"); idx > 0 {
		host = hostPort[:idx]
		port = hostPort[idx+1:]
	}

	conn, err := ssh.Dial("tcp", host+":"+port, config)
	if err != nil {
		return fmt.Errorf("cannot validate mount points: SSH connection failed - %v", err)
	}
	defer conn.Close()

	// Check each mount point path
	for name, mp := range mountPoints {
		if mp.Path == "" {
			continue
		}
		session, err := conn.NewSession()
		if err != nil {
			return fmt.Errorf("cannot validate path for %s: %v", name, err)
		}
		// Check if directory exists and create if not
		err = session.Run(fmt.Sprintf("mkdir -p %s 2>/dev/null && test -d %s", mp.Path, mp.Path))
		session.Close()
		if err != nil {
			return fmt.Errorf("mount point path '%s' for '%s' is not accessible or cannot be created", mp.Path, name)
		}
	}

	return nil
}

// parseSSHKey parses an SSH private key string and returns a signer.
func parseSSHKey(key string) ssh.Signer {
	signer, _ := ssh.ParsePrivateKey([]byte(key))
	return signer
}
