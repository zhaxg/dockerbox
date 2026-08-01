// Package handler provides HTTP handlers for Docker operations.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"dockerbox/backend/internal/model"
	"dockerbox/backend/internal/service"
)

// DockerHandler handles Docker-related HTTP requests.
type DockerHandler struct {
	dockerService    *service.DockerService // default/fallback
	composePaths     []string               // default compose paths
	services         map[string]*service.DockerService // host ID → service
	hostComposePaths map[string][]string    // host ID → compose paths
	hostMountPaths   map[string]string      // host ID → container mount base path (e.g. /opt/docker)
	defaultHostID    string
}

// NewDockerHandler creates a new Docker handler.
func NewDockerHandler(dockerService *service.DockerService, composePaths []string) *DockerHandler {
	if len(composePaths) == 0 {
		composePaths = []string{"/vol1/1000/docker"}
	}
	return &DockerHandler{
		dockerService:    dockerService,
		composePaths:     composePaths,
		services:         make(map[string]*service.DockerService),
		hostComposePaths: make(map[string][]string),
		hostMountPaths:   make(map[string]string),
	}
}

// SetService registers a DockerService for a host ID.
func (h *DockerHandler) SetService(hostID string, svc *service.DockerService) {
	h.services[hostID] = svc
}

// SetComposePaths sets compose scan directories for a specific host.
func (h *DockerHandler) SetComposePaths(hostID string, paths []string) {
	if len(paths) > 0 {
		h.hostComposePaths[hostID] = paths
	}
}

// Services returns the host DockerService map (read-only).
func (h *DockerHandler) Services() map[string]*service.DockerService {
	return h.services
}

// SetDefaultHost sets the default host ID.
func (h *DockerHandler) SetDefaultHost(hostID string) {
	h.defaultHostID = hostID
}

// getService returns the DockerService for the current request's host.
// Requires hostId in request — use requireHostID middleware.
// Returns nil if the requested hostId has no registered service (caller must check).
func (h *DockerHandler) getService(r *http.Request) *service.DockerService {
	hostID := getHostID(r)
	if svc, ok := h.services[hostID]; ok {
		return svc
	}
	// Don't silently fall back to default — return nil so caller can return proper error
	return nil
}

// getComposePaths returns compose scan directories for the current request's host.
func (h *DockerHandler) getComposePaths(r *http.Request) []string {
	hostID := getHostID(r)
	if paths, ok := h.hostComposePaths[hostID]; ok {
		return paths
	}
	return h.composePaths
}

// resolveProjectPath resolves a project ID to its file system path using the request's host service.
func (h *DockerHandler) resolveProjectPath(ctx context.Context, r *http.Request, id string) (string, error) {
	// Check container labels first
	svc := h.getService(r)
	if svc != nil {
		projects, err := svc.ListComposeProjects(ctx)
		if err == nil {
			for _, p := range projects {
				if p.ID == id {
					return p.Path, nil
				}
			}
		}
	}

	// Fall back to compose store
	hostID := getHostID(r)
	if hostID == "" {
		hostID = h.defaultHostID
	}
	store := service.GetComposeStore()
	for _, sp := range store.ListByHost(hostID) {
		if sp.Name == id {
			return sp.Path, nil
		}
	}

	return "", fmt.Errorf("project not found: %s", id)
}

// SetHostMountPath sets the container-side mount base path for a host (for local socket hosts).
func (h *DockerHandler) SetHostMountPath(hostID string, containerPath string) {
	h.hostMountPaths[hostID] = containerPath
}

// translateToContainerPath translates a host path to the container-internal path for local socket hosts.
// Uses mount mappings detected at startup from the container's own mounts (Dockhand-style).
// hostMountPaths stores "source=destination" pairs, e.g. "/vol1/1000/docker=/opt/docker"
func (h *DockerHandler) translateToContainerPath(hostID string, hostPath string) string {
	mountMap, ok := h.hostMountPaths[hostID]
	if !ok || mountMap == "" {
		return hostPath
	}
	parts := strings.SplitN(mountMap, "=", 2)
	if len(parts) != 2 {
		return hostPath
	}
	hostBase := strings.TrimRight(parts[0], "/")
	containerBase := strings.TrimRight(parts[1], "/")
	if strings.HasPrefix(hostPath, hostBase) {
		rel := hostPath[len(hostBase):]
		return containerBase + rel
	}
	return hostPath
}

// RegisterRoutes registers Docker routes on the given router.

// requireHostID middleware enforces that every request carries a valid hostId.
func requireHostID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if getHostID(r) == "" {
			writeError(w, "hostId is required", model.ErrCodeValidationError, http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *DockerHandler) RegisterRoutes(r chi.Router) {
	r.Use(requireHostID)
	// Container routes
	r.Route("/containers", func(r chi.Router) {
		r.Get("/", h.ListContainers)
		r.Get("/stats", h.GetContainerStats)
		r.Get("/{id}", h.GetContainer)
		r.Get("/{id}/inspect", h.InspectContainer)
		r.Post("/{id}/start", h.StartContainer)
		r.Post("/{id}/stop", h.StopContainer)
		r.Post("/{id}/restart", h.RestartContainer)
		r.Get("/{id}/logs", h.GetContainerLogs)
		r.Get("/{id}/exec", h.ExecWebSocket)
	})

	// Image routes
	r.Route("/images", func(r chi.Router) {
		r.Post("/prune", h.PruneImages)
	})

	// Compose routes
	r.Route("/compose", func(r chi.Router) {
		r.Get("/", h.ListComposeProjects)
		r.Get("/available", h.ScanAvailableProjects)
		r.Post("/import", h.ComposeImport)
		r.Get("/check-name", h.CheckComposeName)
		r.Post("/", h.CreateComposeProject)
		r.Post("/{id}/up", h.ComposeUp)
		r.Post("/{id}/down", h.ComposeDown)
		r.Post("/{id}/restart", h.ComposeRestart)
		r.Post("/{id}/redeploy", h.ComposeRedeploy)
		r.Get("/{id}/logs", h.ComposeLogs)
		r.Get("/{id}/file", h.GetComposeFile)
		r.Put("/{id}/file", h.SaveComposeFile)
		r.Delete("/{id}", h.DeleteComposeProject)
		r.Get("/{id}/stream", h.StreamComposeLogs)
		r.Post("/{id}/abort", h.AbortComposeOperation)
		r.Post("/{id}/clean", h.ComposeClean)
	})

	// Network routes
	r.Route("/networks", func(r chi.Router) {
		r.Post("/prune", h.PruneNetworks)
	})
}

// GetHostIP returns the Docker host's IP address.
// ListContainers returns all Docker containers.
func (h *DockerHandler) ListContainers(w http.ResponseWriter, r *http.Request) {
	svc := h.getService(r)
	if svc == nil {
		writeError(w, "Host not found or unavailable", model.ErrCodeNotFound, http.StatusNotFound)
		return
	}
	containers, err := svc.ListContainers(r.Context())
	if err != nil {
		writeError(w, "Failed to list containers", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{"containers": containers}, http.StatusOK)
}

// GetContainerStats returns resource usage stats for all running containers.
func (h *DockerHandler) GetContainerStats(w http.ResponseWriter, r *http.Request) {
	svc := h.getService(r)
	if svc == nil {
		writeError(w, "Host not found or unavailable", model.ErrCodeNotFound, http.StatusNotFound)
		return
	}
	containers, err := svc.ListContainers(r.Context())
	if err != nil {
		writeError(w, "Failed to list containers", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	type containerStats struct {
		ID      string              `json:"id"`
		CPU     float64             `json:"cpu"`
		Memory  model.MemoryUsage   `json:"memory"`
		Network model.NetworkTraffic `json:"network"`
	}

	var results []containerStats
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, c := range containers {
		if c.State == "running" {
			wg.Add(1)
			go func(containerID string) {
				defer wg.Done()
				cpu, mem, net, err := svc.GetStats(r.Context(), containerID)
				if err == nil {
					mu.Lock()
					results = append(results, containerStats{ID: containerID, CPU: cpu, Memory: mem, Network: net})
					mu.Unlock()
				}
			}(c.ID)
		}
	}
	wg.Wait()

	writeJSON(w, map[string]interface{}{"stats": results}, http.StatusOK)
}

// GetContainer returns a single Docker container.
func (h *DockerHandler) GetContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Container ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	svc := h.getService(r)
	if svc == nil {
		writeError(w, "Host not found or unavailable", model.ErrCodeNotFound, http.StatusNotFound)
		return
	}
	container, err := svc.GetContainer(r.Context(), id)
	if err != nil {
		writeError(w, "Failed to get container", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, container, http.StatusOK)
}

// InspectContainer returns detailed container information.
func (h *DockerHandler) InspectContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Container ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	svc := h.getService(r)
	if svc == nil {
		writeError(w, "Host not found or unavailable", model.ErrCodeNotFound, http.StatusNotFound)
		return
	}
	info, err := svc.GetContainer(r.Context(), id)
	if err != nil {
		writeError(w, "Failed to inspect container", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, info, http.StatusOK)
}

// StartContainer starts a Docker container.
func (h *DockerHandler) StartContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Container ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	svc := h.getService(r)
	if svc == nil {
		writeError(w, "Host not found or unavailable", model.ErrCodeNotFound, http.StatusNotFound)
		return
	}
	if err := svc.StartContainer(r.Context(), id); err != nil {
		writeError(w, "Failed to start container", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Container started"}, http.StatusOK)
}

// StopContainer stops a Docker container.
func (h *DockerHandler) StopContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Container ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	svc := h.getService(r)
	if svc == nil {
		writeError(w, "Host not found or unavailable", model.ErrCodeNotFound, http.StatusNotFound)
		return
	}
	if err := svc.StopContainer(r.Context(), id); err != nil {
		writeError(w, "Failed to stop container", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Container stopped"}, http.StatusOK)
}

// RestartContainer restarts a Docker container.
func (h *DockerHandler) RestartContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Container ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	svc := h.getService(r)
	if svc == nil {
		writeError(w, "Host not found or unavailable", model.ErrCodeNotFound, http.StatusNotFound)
		return
	}
	if err := svc.RestartContainer(r.Context(), id); err != nil {
		writeError(w, "Failed to restart container", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Container restarted"}, http.StatusOK)
}

// KillContainer kills a Docker container.
// GetContainerLogs returns logs for a Docker container.
func (h *DockerHandler) GetContainerLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Container ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	tail := 100
	if t := r.URL.Query().Get("tail"); t != "" {
		if parsed, err := strconv.Atoi(t); err == nil {
			tail = parsed
		}
	}

	svc := h.getService(r)
	if svc == nil {
		writeError(w, "Host not found or unavailable", model.ErrCodeNotFound, http.StatusNotFound)
		return
	}
	logs, err := svc.GetContainerLogs(r.Context(), id, tail)
	if err != nil {
		writeError(w, "Failed to get container logs", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{"logs": logs}, http.StatusOK)
}

// GetStats returns resource usage statistics for a container.
// PruneImages removes unused Docker images.
func (h *DockerHandler) PruneImages(w http.ResponseWriter, r *http.Request) {
	svc := h.getService(r)
	if svc == nil {
		writeError(w, "Host not found or unavailable", model.ErrCodeNotFound, http.StatusNotFound)
		return
	}
	reclaimed, err := svc.PruneImages(r.Context())
	if err != nil {
		writeError(w, "Failed to prune images", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	deleted := 0
	if reclaimed.ImagesDeleted != nil {
		deleted = len(reclaimed.ImagesDeleted)
	}
	spaceMB := reclaimed.SpaceReclaimed / 1024 / 1024

	writeJSON(w, map[string]interface{}{
		"deleted": deleted,
		"spaceMB": spaceMB,
		"message": fmt.Sprintf("deleted %d images, freed %dMB", deleted, spaceMB),
	}, http.StatusOK)
}

// PruneNetworks removes unused Docker networks.
func (h *DockerHandler) PruneNetworks(w http.ResponseWriter, r *http.Request) {
	svc := h.getService(r)
	if svc == nil {
		writeError(w, "Host not found or unavailable", model.ErrCodeNotFound, http.StatusNotFound)
		return
	}
	reclaimed, err := svc.PruneNetworks(r.Context())
	if err != nil {
		writeError(w, "Failed to prune networks", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"deleted": reclaimed,
		"message": fmt.Sprintf("deleted %d networks", reclaimed),
	}, http.StatusOK)
}

// ExecWebSocket handles WebSocket terminal connections.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (h *DockerHandler) ExecWebSocket(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Container ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	_, msg, err := conn.ReadMessage()
	if err != nil {
		return
	}

	var authMsg struct {
		Type  string `json:"type"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(msg, &authMsg); err != nil || authMsg.Type != "auth" {
		conn.WriteMessage(websocket.TextMessage, []byte("Error: invalid auth message"))
		return
	}

	if authMsg.Token == "" {
		conn.WriteMessage(websocket.TextMessage, []byte("Error: no token provided"))
		return
	}

	svc := h.getService(r)
	if svc == nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Error: host not found or unavailable"))
		return
	}
	shell := svc.DetectShell(r.Context(), id)
	execID, err := svc.CreateExec(r.Context(), id, []string{shell, "-i"})
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31m错误: 无法创建exec: "+err.Error()+"\x1b[0m\r\n"))
		conn.WriteMessage(websocket.TextMessage, []byte("\x1b[33m此容器可能没有可用的shell工具\x1b[0m\r\n"))
		return
	}

	hijack, err := svc.StartExec(r.Context(), execID)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Error: "+err.Error()))
		return
	}
	defer hijack.Close()

	var mu sync.Mutex

	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			mu.Lock()
			hijack.Conn.Write(msg)
			mu.Unlock()
		}
	}()

	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := hijack.Reader.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				mu.Lock()
				conn.WriteMessage(websocket.BinaryMessage, buf[:n])
				mu.Unlock()
			}
		}
	}()

	select {}
}

// CheckComposeName checks if a project name already exists for the current host.
