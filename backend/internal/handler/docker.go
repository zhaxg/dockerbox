// Package handler provides HTTP handlers for Docker operations.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/jR4dh3y/BoxBox/backend/internal/model"
	"github.com/jR4dh3y/BoxBox/backend/internal/service"
)

// DockerHandler handles Docker-related HTTP requests.
type DockerHandler struct {
	dockerService    *service.DockerService // default/fallback
	composePaths     []string               // default compose paths
	services         map[string]*service.DockerService // host ID → service
	hostComposePaths map[string][]string    // host ID → compose paths
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
func (h *DockerHandler) getService(r *http.Request) *service.DockerService {
	hostID := getHostID(r)
	if svc, ok := h.services[hostID]; ok {
		return svc
	}
	return h.dockerService
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
	projects, err := h.getService(r).ListComposeProjects(ctx)
	if err == nil {
		for _, p := range projects {
			if p.ID == id {
				return p.Path, nil
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
		r.Get("/host-ip", h.GetHostIP)
		r.Get("/stats", h.GetStats)
		r.Get("/{id}", h.GetContainer)
		r.Get("/{id}/inspect", h.InspectContainer)
		r.Post("/{id}/start", h.StartContainer)
		r.Post("/{id}/stop", h.StopContainer)
		r.Post("/{id}/restart", h.RestartContainer)
		r.Post("/{id}/kill", h.KillContainer)
		r.Delete("/{id}", h.DeleteContainer)
		r.Get("/{id}/logs", h.GetContainerLogs)
		r.Get("/{id}/exec", h.ExecWebSocket)
	})

	// Image routes
	r.Route("/images", func(r chi.Router) {
		r.Get("/", h.ListImages)
		r.Delete("/{id}", h.DeleteImage)
		r.Post("/pull", h.PullImage)
		r.Post("/prune", h.PruneImages)
	})

	// Compose routes
	r.Route("/compose", func(r chi.Router) {
		r.Get("/", h.ListComposeProjects)
		r.Get("/available", h.ScanAvailableProjects)
		r.Post("/import", h.ImportComposeProjects)
		r.Get("/check-name", h.CheckComposeName)
		r.Post("/", h.CreateComposeProject)
		r.Post("/{id}/up", h.ComposeUp)
		r.Post("/{id}/down", h.ComposeDown)
		r.Post("/{id}/build", h.ComposeBuild)
		r.Post("/{id}/restart", h.ComposeRestart)
		r.Post("/{id}/pull", h.ComposePull)
		r.Post("/{id}/redeploy", h.ComposeRedeploy)
		r.Post("/{id}/rebuild", h.ComposeRebuild)
		r.Get("/{id}/logs", h.ComposeLogs)
		r.Get("/{id}/file", h.GetComposeFile)
		r.Put("/{id}/file", h.SaveComposeFile)
		r.Get("/{id}/env", h.GetComposeEnv)
		r.Put("/{id}/env", h.SaveComposeEnv)
		r.Delete("/{id}", h.DeleteComposeProject)
		r.Get("/{id}/stream", h.StreamComposeLogs)
		r.Post("/{id}/abort", h.AbortComposeOperation)
		r.Post("/{id}/clean", h.ComposeClean)
		r.Get("/{id}/status", h.GetComposeStatus)
	})

	// Network routes
	r.Route("/networks", func(r chi.Router) {
		r.Get("/", h.ListNetworks)
		r.Delete("/{id}", h.RemoveNetwork)
		r.Post("/prune", h.PruneNetworks)
	})
}

// GetHostIP returns the Docker host's IP address.
func (h *DockerHandler) GetHostIP(w http.ResponseWriter, r *http.Request) {
	hostIP := h.getService(r).GetHostIP(r.Context())
	writeJSON(w, map[string]string{"ip": hostIP}, http.StatusOK)
}

// ListContainers returns all Docker containers.
func (h *DockerHandler) ListContainers(w http.ResponseWriter, r *http.Request) {
	containers, err := h.getService(r).ListContainers(r.Context())
	if err != nil {
		writeError(w, "Failed to list containers", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	var wg sync.WaitGroup
	for i := range containers {
		if containers[i].State == "running" {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				cpu, mem, net, err := h.getService(r).GetStats(r.Context(), containers[idx].ID)
				if err == nil {
					containers[idx].CPU = cpu
					containers[idx].Memory = mem
					containers[idx].Network = net
				}
			}(i)
		}
	}
	wg.Wait()

	writeJSON(w, map[string]interface{}{"containers": containers}, http.StatusOK)
}

// GetContainer returns a single Docker container.
func (h *DockerHandler) GetContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Container ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	container, err := h.getService(r).GetContainer(r.Context(), id)
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

	info, err := h.getService(r).GetContainer(r.Context(), id)
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

	if err := h.getService(r).StartContainer(r.Context(), id); err != nil {
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

	if err := h.getService(r).StopContainer(r.Context(), id); err != nil {
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

	if err := h.getService(r).RestartContainer(r.Context(), id); err != nil {
		writeError(w, "Failed to restart container", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Container restarted"}, http.StatusOK)
}

// KillContainer kills a Docker container.
func (h *DockerHandler) KillContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Container ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	var req struct {
		Signal string `json:"signal"`
	}
	// signal is optional; default to SIGKILL
	signal := "SIGKILL"
	if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.Signal != "" {
		signal = req.Signal
	}

	if err := h.getService(r).KillContainer(r.Context(), id, signal); err != nil {
		writeError(w, "Failed to kill container", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Container killed"}, http.StatusOK)
}

// DeleteContainer removes a Docker container.
func (h *DockerHandler) DeleteContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Container ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if err := h.getService(r).DeleteContainer(r.Context(), id); err != nil {
		writeError(w, "Failed to delete container", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Container deleted"}, http.StatusOK)
}

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

	logs, err := h.getService(r).GetContainerLogs(r.Context(), id, tail)
	if err != nil {
		writeError(w, "Failed to get container logs", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{"logs": logs}, http.StatusOK)
}

// GetStats returns resource usage statistics for a container.
func (h *DockerHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	// Return empty stats - real stats are fetched per-container in ListContainers
	writeJSON(w, map[string]interface{}{}, http.StatusOK)
}

// ListImages returns all Docker images.
// ListComposeProjects returns all compose projects for the current host.
// ListNetworks returns all Docker networks.
func (h *DockerHandler) ListNetworks(w http.ResponseWriter, r *http.Request) {
	networks, err := h.getService(r).ListNetworks(r.Context())
	if err != nil {
		writeError(w, "Failed to list networks", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"networks": networks}, http.StatusOK)
}

// RemoveNetwork removes a Docker network.
func (h *DockerHandler) RemoveNetwork(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Network ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if err := h.getService(r).RemoveNetwork(r.Context(), id); err != nil {
		writeError(w, "Failed to remove network", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Network removed"}, http.StatusOK)
}

// PruneNetworks removes unused Docker networks.
func (h *DockerHandler) PruneNetworks(w http.ResponseWriter, r *http.Request) {
	reclaimed, err := h.getService(r).PruneNetworks(r.Context())
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
