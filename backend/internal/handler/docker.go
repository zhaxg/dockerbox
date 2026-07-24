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
	dockerService *service.DockerService
	composePaths  []string
}

// NewDockerHandler creates a new Docker handler.
func NewDockerHandler(dockerService *service.DockerService, composePaths []string) *DockerHandler {
	if len(composePaths) == 0 {
		composePaths = []string{"/vol1/1000/docker"}
	}
	return &DockerHandler{
		dockerService: dockerService,
		composePaths:  composePaths,
	}
}

// RegisterRoutes registers Docker routes on the given router.
func (h *DockerHandler) RegisterRoutes(r chi.Router) {
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
		r.Post("/", h.CreateComposeProject)
		r.Post("/{id}/up", h.ComposeUp)
		r.Post("/{id}/down", h.ComposeDown)
		r.Post("/{id}/build", h.ComposeBuild)
		r.Post("/{id}/restart", h.ComposeRestart)
		r.Post("/{id}/pull", h.ComposePull)
		r.Post("/{id}/redeploy", h.ComposeRedeploy)
		r.Get("/{id}/logs", h.ComposeLogs)
		r.Get("/{id}/file", h.GetComposeFile)
		r.Put("/{id}/file", h.SaveComposeFile)
		r.Get("/{id}/env", h.GetComposeEnv)
		r.Put("/{id}/env", h.SaveComposeEnv)
		r.Delete("/{id}", h.DeleteComposeProject)
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
	hostIP := h.dockerService.GetHostIP(r.Context())
	writeJSON(w, map[string]string{"ip": hostIP}, http.StatusOK)
}

// ListContainers returns all Docker containers.
func (h *DockerHandler) ListContainers(w http.ResponseWriter, r *http.Request) {
	containers, err := h.dockerService.ListContainers(r.Context())
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
				cpu, mem, net, err := h.dockerService.GetStats(r.Context(), containers[idx].ID)
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

	container, err := h.dockerService.GetContainer(r.Context(), id)
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

	info, err := h.dockerService.GetContainer(r.Context(), id)
	if err != nil {
		writeError(w, "Failed to inspect container", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, info, http.StatusOK)
}

// ListImages returns all Docker images.
func (h *DockerHandler) ListImages(w http.ResponseWriter, r *http.Request) {
	images, err := h.dockerService.ListImages(r.Context())
	if err != nil {
		writeError(w, "Failed to list images", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{"images": images}, http.StatusOK)
}

// DeleteImage removes a Docker image.
func (h *DockerHandler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Image ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if err := h.dockerService.DeleteImage(r.Context(), id); err != nil {
		writeError(w, "Failed to delete image", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Image deleted"}, http.StatusOK)
}

// PullImage pulls a Docker image.
func (h *DockerHandler) PullImage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Image string `json:"image"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if req.Image == "" {
		writeError(w, "Image name is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if err := h.dockerService.PullImage(r.Context(), req.Image); err != nil {
		writeError(w, "Failed to pull image", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Image pulled successfully"}, http.StatusOK)
}

// PruneImages removes unused Docker images.
func (h *DockerHandler) PruneImages(w http.ResponseWriter, r *http.Request) {
	report, err := h.dockerService.PruneImages(r.Context())
	if err != nil {
		writeError(w, "Failed to prune images", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"reclaimed": report.ImagesDeleted, "space": report.SpaceReclaimed}, http.StatusOK)
}

// GetStats returns Docker system statistics.
func (h *DockerHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.dockerService.GetDockerStats(r.Context())
	if err != nil {
		writeError(w, "Failed to get Docker stats", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, stats, http.StatusOK)
}

// StartContainer starts a Docker container.
func (h *DockerHandler) StartContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Container ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if err := h.dockerService.StartContainer(r.Context(), id); err != nil {
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

	if err := h.dockerService.StopContainer(r.Context(), id); err != nil {
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

	if err := h.dockerService.RestartContainer(r.Context(), id); err != nil {
		writeError(w, "Failed to restart container", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Container restarted"}, http.StatusOK)
}

// KillContainer sends a signal to a container.
func (h *DockerHandler) KillContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Container ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	signal := r.URL.Query().Get("signal")
	if signal == "" {
		signal = "SIGKILL"
	}

	if err := h.dockerService.KillContainer(r.Context(), id, signal); err != nil {
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

	if err := h.dockerService.DeleteContainer(r.Context(), id); err != nil {
		writeError(w, "Failed to delete container", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Container deleted"}, http.StatusOK)
}

// GetContainerLogs returns container logs.
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

	logs, err := h.dockerService.GetContainerLogs(r.Context(), id, tail)
	if err != nil {
		writeError(w, "Failed to get container logs", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{"lines": logs}, http.StatusOK)
}

// ListComposeProjects returns all compose projects.
func (h *DockerHandler) ListComposeProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.dockerService.ListComposeProjects(r.Context(), h.composePaths)
	if err != nil {
		writeError(w, "Failed to list compose projects", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{"projects": projects}, http.StatusOK)
}

// CreateComposeProject creates a new compose project.
func (h *DockerHandler) CreateComposeProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name            string `json:"name"`
		ComposeContent  string `json:"composeContent"`
		EnvContent      string `json:"envContent"`
		BasePath        string `json:"basePath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		writeError(w, "Project name is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if req.ComposeContent == "" {
		writeError(w, "Compose content is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	// Default base path
	if req.BasePath == "" {
		req.BasePath = "/vol1/1000/docker"
	}

	result, err := h.dockerService.CreateComposeProject(r.Context(), req.Name, req.ComposeContent, req.EnvContent, req.BasePath)
	if err != nil {
		writeError(w, result.Message, model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, result, http.StatusOK)
}

// ComposeUp starts a compose project.
func (h *DockerHandler) ComposeUp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	result, err := h.dockerService.ComposeUp(r.Context(), path)
	if err != nil {
		writeError(w, result.Message, model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, result, http.StatusOK)
}

// ComposeDown stops a compose project.
func (h *DockerHandler) ComposeDown(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	result, err := h.dockerService.ComposeDown(r.Context(), path)
	if err != nil {
		writeError(w, result.Message, model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, result, http.StatusOK)
}

// ComposeBuild builds a compose project.
func (h *DockerHandler) ComposeBuild(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	result, err := h.dockerService.ComposeBuild(r.Context(), path)
	if err != nil {
		writeError(w, result.Message, model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, result, http.StatusOK)
}

// ComposeRestart restarts a compose project.
func (h *DockerHandler) ComposeRestart(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	result, err := h.dockerService.ComposeRestart(r.Context(), path)
	if err != nil {
		writeError(w, result.Message, model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, result, http.StatusOK)
}

// ComposePull pulls images for a compose project.
func (h *DockerHandler) ComposePull(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	result, err := h.dockerService.ComposePull(r.Context(), path)
	if err != nil {
		writeError(w, result.Message, model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, result, http.StatusOK)
}

// ComposeRedeploy runs docker-compose down then up.
func (h *DockerHandler) ComposeRedeploy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	// First down
	downResult, err := h.dockerService.ComposeDown(r.Context(), path)
	if err != nil {
		writeError(w, downResult.Message, model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	// Then up
	upResult, err := h.dockerService.ComposeUp(r.Context(), path)
	if err != nil {
		writeError(w, upResult.Message, model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Project redeployed"}, http.StatusOK)
}

// ComposeLogs returns compose project logs.
func (h *DockerHandler) ComposeLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	tail := 100
	if t := r.URL.Query().Get("tail"); t != "" {
		if parsed, err := strconv.Atoi(t); err == nil {
			tail = parsed
		}
	}

	logs, err := h.dockerService.ComposeLogs(r.Context(), path, tail)
	if err != nil {
		writeError(w, "Failed to get compose logs", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{"lines": logs}, http.StatusOK)
}

// GetComposeFile returns the docker-compose.yml content.
func (h *DockerHandler) GetComposeFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	content, err := h.dockerService.GetComposeFile(r.Context(), path)
	if err != nil {
		writeError(w, "Failed to get compose file", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"content": content}, http.StatusOK)
}

// SaveComposeFile saves the docker-compose.yml content.
func (h *DockerHandler) SaveComposeFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if err := h.dockerService.SaveComposeFile(path, req.Content); err != nil {
		writeError(w, "Failed to save compose file", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Compose file saved"}, http.StatusOK)
}

// GetComposeEnv returns the .env content.
func (h *DockerHandler) GetComposeEnv(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	content, err := h.dockerService.GetComposeEnv(r.Context(), path)
	if err != nil {
		writeError(w, "Failed to get env file", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"content": content}, http.StatusOK)
}

// SaveComposeEnv saves the .env content.
func (h *DockerHandler) SaveComposeEnv(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if err := h.dockerService.SaveComposeEnv(path, req.Content); err != nil {
		writeError(w, "Failed to save env file", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Env file saved"}, http.StatusOK)
}

// DeleteComposeProject removes a compose project and its files.
func (h *DockerHandler) DeleteComposeProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if err := h.dockerService.DeleteComposeProject(path); err != nil {
		writeError(w, "Failed to delete project", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Project deleted"}, http.StatusOK)
}

// ListNetworks returns all Docker networks.
func (h *DockerHandler) ListNetworks(w http.ResponseWriter, r *http.Request) {
	networks, err := h.dockerService.ListNetworks(r.Context())
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

	if err := h.dockerService.RemoveNetwork(r.Context(), id); err != nil {
		writeError(w, "Failed to remove network", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Network removed"}, http.StatusOK)
}

// PruneNetworks removes unused Docker networks.
func (h *DockerHandler) PruneNetworks(w http.ResponseWriter, r *http.Request) {
	reclaimed, err := h.dockerService.PruneNetworks(r.Context())
	if err != nil {
		writeError(w, "Failed to prune networks", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"reclaimed": reclaimed}, http.StatusOK)
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

	shell := h.dockerService.DetectShell(r.Context(), id)
	execID, err := h.dockerService.CreateExec(r.Context(), id, []string{shell})
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Error: "+err.Error()))
		return
	}

	hijack, err := h.dockerService.StartExec(r.Context(), execID)
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

// resolveProjectPath resolves a project ID to its file system path.
func (h *DockerHandler) resolveProjectPath(ctx context.Context, id string) (string, error) {
	projects, err := h.dockerService.ListComposeProjects(ctx, h.composePaths)
	if err != nil {
		return "", fmt.Errorf("failed to list projects: %w", err)
	}

	for _, p := range projects {
		if p.ID == id {
			return p.Path, nil
		}
	}

	return "", fmt.Errorf("project not found: %s", id)
}
