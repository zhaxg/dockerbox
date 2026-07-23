// Package handler provides HTTP handlers for Docker network operations.
package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jR4dh3y/BoxBox/backend/internal/model"
)

// NetworkHandler handles Docker network-related HTTP requests.
type NetworkHandler struct {
	dockerService DockerService
}

// DockerService interface for network operations.
type DockerService interface {
	ListNetworks(ctx interface{ Value(any) any }) ([]model.Network, error)
	RemoveNetwork(ctx interface{ Value(any) any }, id string) error
	PruneNetworks(ctx interface{ Value(any) any }) (int64, error)
}

// NewNetworkHandler creates a new network handler.
func NewNetworkHandler(dockerService DockerService) *NetworkHandler {
	return &NetworkHandler{dockerService: dockerService}
}

// RegisterRoutes registers network routes on the given router.
func (h *NetworkHandler) RegisterRoutes(r chi.Router) {
	r.Get("/", h.ListNetworks)
	r.Delete("/{id}", h.RemoveNetwork)
	r.Post("/prune", h.PruneNetworks)
}

// ListNetworks returns all Docker networks.
func (h *NetworkHandler) ListNetworks(w http.ResponseWriter, r *http.Request) {
	networks, err := h.dockerService.ListNetworks(r.Context())
	if err != nil {
		writeError(w, "Failed to list networks", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"networks": networks}, http.StatusOK)
}

// RemoveNetwork removes a Docker network.
func (h *NetworkHandler) RemoveNetwork(w http.ResponseWriter, r *http.Request) {
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
func (h *NetworkHandler) PruneNetworks(w http.ResponseWriter, r *http.Request) {
	reclaimed, err := h.dockerService.PruneNetworks(r.Context())
	if err != nil {
		writeError(w, "Failed to prune networks", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"reclaimed": reclaimed}, http.StatusOK)
}
