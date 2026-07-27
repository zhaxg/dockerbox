// Package handler provides HTTP handlers for Docker image operations.
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"dockerbox/backend/internal/model"
)

func (h *DockerHandler) ListImages(w http.ResponseWriter, r *http.Request) {
	images, err := h.getService(r).ListImages(r.Context())
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

	if err := h.getService(r).DeleteImage(r.Context(), id); err != nil {
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

	if err := h.getService(r).PullImage(r.Context(), req.Image); err != nil {
		writeError(w, "Failed to pull image", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Image pulled"}, http.StatusOK)
}

// PruneImages removes unused Docker images.
func (h *DockerHandler) PruneImages(w http.ResponseWriter, r *http.Request) {
	reclaimed, err := h.getService(r).PruneImages(r.Context())
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
		"deleted":  deleted,
		"spaceMB":  spaceMB,
		"message":  fmt.Sprintf("deleted %d images, freed %dMB", deleted, spaceMB),
	}, http.StatusOK)
}

