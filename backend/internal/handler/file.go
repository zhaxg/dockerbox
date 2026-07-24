package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"strings"
	"net/url"
	"github.com/go-chi/chi/v5"
	"github.com/jR4dh3y/BoxBox/backend/internal/model"
	"github.com/jR4dh3y/BoxBox/backend/internal/pkg/validator"
	"github.com/jR4dh3y/BoxBox/backend/internal/service"
)

// FileHandler handles file-related HTTP requests
type FileHandler struct {
	fileService     service.FileService
	hostAccess      map[string]service.HostFileAccess // host ID → file access impl
	hostMountPoints map[string][]model.MountPoint
	defaultHostID   string
}

// NewFileHandler creates a new file handler
func NewFileHandler(fileService service.FileService) *FileHandler {
	return &FileHandler{
		fileService:     fileService,
		hostAccess:      make(map[string]service.HostFileAccess),
		hostMountPoints: make(map[string][]model.MountPoint),
	}
}

// SetMountPoints registers mount points for a host ID.
func (h *FileHandler) SetMountPoints(hostID string, mps []model.MountPoint) {
	h.hostMountPoints[hostID] = mps
}

// SetHostAccess registers a HostFileAccess implementation for a host ID.
func (h *FileHandler) SetHostAccess(hostID string, access service.HostFileAccess) {
	h.hostAccess[hostID] = access
}

// SetDefaultHost sets the default host ID.
func (h *FileHandler) SetDefaultHost(hostID string) {
	h.defaultHostID = hostID
}

// getMountPoints returns mount points for the current request's host.
func (h *FileHandler) getMountPoints(r *http.Request) []model.MountPoint {
	hostID := r.Header.Get("X-Host-ID")
	if hostID != "" {
		if mps, ok := h.hostMountPoints[hostID]; ok {
			return mps
		}
	}
	if h.defaultHostID != "" {
		if mps, ok := h.hostMountPoints[h.defaultHostID]; ok {
			return mps
		}
	}
	return h.fileService.ListMountPoints()
}

// getHostAccess returns the HostFileAccess for the current request's host, or nil for local.
func (h *FileHandler) getHostAccess(r *http.Request) service.HostFileAccess {
	hostID := r.Header.Get("X-Host-ID")
	if hostID != "" {
		if access, ok := h.hostAccess[hostID]; ok {
			return access
		}
	}
	if h.defaultHostID != "" {
		if access, ok := h.hostAccess[h.defaultHostID]; ok {
			return access
		}
	}
	return nil
}

// resolvePath resolves a BoxBox browse path to the actual host filesystem path.
func (h *FileHandler) ResolvePath(r *http.Request, boxPath string) string {
	decoded, _ := url.PathUnescape(boxPath)
	parts := strings.SplitN(decoded, "/", 2)
	mountName := parts[0]
	subPath := ""
	if len(parts) > 1 {
		subPath = parts[1]
	}

	for _, mp := range h.getMountPoints(r) {
		if mp.Name == mountName {
			if subPath != "" {
				return mp.Path + "/" + subPath
			}
			return mp.Path
		}
	}
	return "/" + decoded
}

// RegisterRoutes registers file routes on the given router
func (h *FileHandler) RegisterRoutes(r chi.Router) {
	r.Get("/", h.ListRoots)
	r.Get("/stats", h.GetDriveStats)
	r.Get("/*", h.GetPath)
	r.Post("/*", h.CreateItem)
	r.Put("/*", h.Rename)
	r.Patch("/*", h.SaveFileContent)
	r.Delete("/*", h.Delete)
}

// MountPointResponse represents a mount point in API responses
type MountPointResponse struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	ReadOnly bool   `json:"readOnly"`
}

// RootsResponse represents the list of mount points
type RootsResponse struct {
	Roots []MountPointResponse `json:"roots"`
}

// CreateItemRequest represents the create file/directory request body
type CreateItemRequest struct {
	Name    string `json:"name"`
	Type    string `json:"type,omitempty"`
	Content string `json:"content,omitempty"`
}

// RenameRequest represents the rename request body
type RenameRequest struct {
	NewPath string `json:"newPath"`
}

// SaveFileRequest represents the file content save request body
type SaveFileRequest struct {
	Content string `json:"content"`
}

// ListRoots returns all configured mount points
// GET /api/v1/files
func (h *FileHandler) ListRoots(w http.ResponseWriter, r *http.Request) {
	mounts := h.getMountPoints(r)

	roots := make([]MountPointResponse, len(mounts))
	for i, mount := range mounts {
		roots[i] = MountPointResponse{
			Name:     mount.Name,
			Path:     mount.Path,
			ReadOnly: mount.ReadOnly,
		}
	}

	writeJSON(w, RootsResponse{Roots: roots}, http.StatusOK)
}

// GetDriveStats returns disk usage statistics for all mount points
// GET /api/v1/files/stats
func (h *FileHandler) GetDriveStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.fileService.GetDriveStats(r.Context())
	if err != nil {
		HandleServiceError(w, err)
		return
	}

	writeJSON(w, stats, http.StatusOK)
}

// GetPath handles GET requests for a path - returns directory listing or file info
// GET /api/v1/files/*path
func (h *FileHandler) GetPath(w http.ResponseWriter, r *http.Request) {
	path := chi.URLParam(r, "*")
	if path == "" {
		h.ListRoots(w, r)
		return
	}

	// Remote/socket host: use HostFileAccess
	if access := h.getHostAccess(r); access != nil {
		h.getPathRemote(w, r, path, access)
		return
	}

	// Local: use file service
	info, err := h.fileService.GetInfo(r.Context(), path)
	if err != nil {
		HandleServiceError(w, err)
		return
	}

	if info.IsDir {
		list, err := h.fileService.List(r.Context(), path, h.parseListOptions(r))
		if err != nil {
			HandleServiceError(w, err)
			return
		}
		writeJSON(w, list, http.StatusOK)
	} else {
		writeJSON(w, info, http.StatusOK)
	}
}

// getPathRemote handles file listing for remote hosts via HostFileAccess.
func (h *FileHandler) getPathRemote(w http.ResponseWriter, r *http.Request, boxPath string, access service.HostFileAccess) {
	hostPath := h.ResolvePath(r, boxPath)

	fileInfo, err := access.GetInfo(r.Context(), hostPath)
	if err != nil {
		HandleServiceError(w, err)
		return
	}

	if fileInfo.IsDir {
		files, err := access.List(r.Context(), hostPath)
		if err != nil {
			HandleServiceError(w, err)
			return
		}
		for i := range files {
			files[i].Path = boxPath + "/" + files[i].Name
		}
		writeJSON(w, model.FileList{
			Path:       boxPath,
			Items:      files,
			TotalCount: len(files),
		}, http.StatusOK)
	} else {
		fileInfo.Path = boxPath
		writeJSON(w, fileInfo, http.StatusOK)
	}
}

// CreateItem creates a new directory or file
// POST /api/v1/files/*path
func (h *FileHandler) CreateItem(w http.ResponseWriter, r *http.Request) {
	basePath := chi.URLParam(r, "*")

	var req CreateItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	itemType := req.Type
	if itemType == "" {
		itemType = "directory"
	}

	if req.Name == "" {
		writeError(w, "Name is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if !validator.IsValidFileName(req.Name) {
		writeError(w, "Name cannot contain path separators or special path names", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	// Build full path
	fullPath := basePath
	if fullPath != "" {
		fullPath = fullPath + "/" + req.Name
	} else {
		fullPath = req.Name
	}

	// Remote/socket host: use HostFileAccess
	if access := h.getHostAccess(r); access != nil {
		hostPath := h.ResolvePath(r, fullPath)
		switch itemType {
		case "directory", "folder":
			if err := access.Mkdir(r.Context(), hostPath); err != nil {
				HandleServiceError(w, err)
				return
			}
		case "file":
			content := []byte(req.Content)
			if err := access.Write(r.Context(), hostPath, content); err != nil {
				HandleServiceError(w, err)
				return
			}
		default:
			writeError(w, "Type must be file or directory", model.ErrCodeValidationError, http.StatusBadRequest)
			return
		}
		info, infoErr := access.GetInfo(r.Context(), hostPath)
		if infoErr != nil {
			writeJSON(w, map[string]string{"message": "Created successfully"}, http.StatusCreated)
			return
		}
		info.Path = fullPath
		writeJSON(w, info, http.StatusCreated)
		return
	}

	switch itemType {
	case "directory", "folder":
		// Create directory
		if err := h.fileService.CreateDir(r.Context(), fullPath); err != nil {
			HandleServiceError(w, err)
			return
		}
	case "file":
		// Create file without overwriting any existing item
		file, err := h.fileService.CreateFile(r.Context(), fullPath)
		if err != nil {
			HandleServiceError(w, err)
			return
		}

		if req.Content != "" {
			if _, err := io.WriteString(file, req.Content); err != nil {
				_ = file.Close()
				_ = h.fileService.Delete(r.Context(), fullPath)
				writeError(w, "Failed to write file content", model.ErrCodeInternalError, http.StatusInternalServerError)
				return
			}
		}

		if err := file.Close(); err != nil {
			_ = h.fileService.Delete(r.Context(), fullPath)
			writeError(w, "Failed to create file", model.ErrCodeInternalError, http.StatusInternalServerError)
			return
		}
	default:
		writeError(w, "Type must be file or directory", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	// Return the created item info
	info, err := h.fileService.GetInfo(r.Context(), fullPath)
	if err != nil {
		HandleServiceError(w, err)
		return
	}

	writeJSON(w, info, http.StatusCreated)
}

// SaveFileContent overwrites an existing file's content
// PATCH /api/v1/files/*path
func (h *FileHandler) SaveFileContent(w http.ResponseWriter, r *http.Request) {
	path := chi.URLParam(r, "*")
	if path == "" {
		writeError(w, "Path is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	var req SaveFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	// Remote/socket host: use HostFileAccess
	if access := h.getHostAccess(r); access != nil {
		hostPath := h.ResolvePath(r, path)
		if err := access.Write(r.Context(), hostPath, []byte(req.Content)); err != nil {
			HandleServiceError(w, err)
			return
		}
		info, err := access.GetInfo(r.Context(), hostPath)
		if err != nil {
			writeJSON(w, &model.FileInfo{Name: path[strings.LastIndex(path, "/")+1:], Path: path, Size: int64(len(req.Content))}, http.StatusOK)
			return
		}
		info.Path = path
		writeJSON(w, info, http.StatusOK)
		return
	}

	if err := h.fileService.WriteFile(r.Context(), path, []byte(req.Content)); err != nil {
		HandleServiceError(w, err)
		return
	}

	info, err := h.fileService.GetInfo(r.Context(), path)
	if err != nil {
		HandleServiceError(w, err)
		return
	}

	writeJSON(w, info, http.StatusOK)
}

// Rename renames/moves a file or directory
// PUT /api/v1/files/*path
func (h *FileHandler) Rename(w http.ResponseWriter, r *http.Request) {
	oldPath := chi.URLParam(r, "*")
	if oldPath == "" {
		writeError(w, "Path is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	var req RenameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	// Validate new path
	if req.NewPath == "" {
		writeError(w, "New path is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	// Remote/socket host: use HostFileAccess
	if access := h.getHostAccess(r); access != nil {
		oldHostPath := h.ResolvePath(r, oldPath)
		newHostPath := h.ResolvePath(r, req.NewPath)
		if err := access.Rename(r.Context(), oldHostPath, newHostPath); err != nil {
			HandleServiceError(w, err)
			return
		}
		info, infoErr := access.GetInfo(r.Context(), newHostPath)
		if infoErr != nil {
			writeJSON(w, map[string]string{"message": "Renamed successfully"}, http.StatusOK)
			return
		}
		info.Path = req.NewPath
		writeJSON(w, info, http.StatusOK)
		return
	}

	// Perform rename
	if err := h.fileService.Rename(r.Context(), oldPath, req.NewPath); err != nil {
		HandleServiceError(w, err)
		return
	}

	// Return the renamed file/directory info
	info, err := h.fileService.GetInfo(r.Context(), req.NewPath)
	if err != nil {
		HandleServiceError(w, err)
		return
	}

	writeJSON(w, info, http.StatusOK)
}

// Delete removes a file or directory
// DELETE /api/v1/files/*path
func (h *FileHandler) Delete(w http.ResponseWriter, r *http.Request) {
	path := chi.URLParam(r, "*")
	if path == "" {
		writeError(w, "Path is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	// Remote/socket host: use HostFileAccess
	if access := h.getHostAccess(r); access != nil {
		hostPath := h.ResolvePath(r, path)
		if err := access.Remove(r.Context(), hostPath); err != nil {
			HandleServiceError(w, err)
			return
		}
		writeJSON(w, map[string]string{"message": "Deleted successfully"}, http.StatusOK)
		return
	}

	// Check if confirmation is required (query param)
	confirm := r.URL.Query().Get("confirm")
	if confirm != "true" {
		// Check if path exists and get info
		info, err := h.fileService.GetInfo(r.Context(), path)
		if err != nil {
			HandleServiceError(w, err)
			return
		}

		// If it's a directory, require confirmation
		if info.IsDir {
			writeError(w, "Confirmation required to delete directory. Add ?confirm=true to confirm.", model.ErrCodeValidationError, http.StatusBadRequest)
			return
		}
	}

	// Perform delete
	if err := h.fileService.Delete(r.Context(), path); err != nil {
		HandleServiceError(w, err)
		return
	}

	writeJSON(w, map[string]string{"message": "Deleted successfully"}, http.StatusOK)
}

// parseListOptions extracts listing options from query parameters
func (h *FileHandler) parseListOptions(r *http.Request) model.ListOptions {
	opts := model.DefaultListOptions()

	if page := r.URL.Query().Get("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil && p > 0 {
			opts.Page = p
		}
	}

	if pageSize := r.URL.Query().Get("pageSize"); pageSize != "" {
		if ps, err := strconv.Atoi(pageSize); err == nil && ps > 0 && ps <= 1000 {
			opts.PageSize = ps
		}
	}

	if includeHidden := r.URL.Query().Get("includeHidden"); includeHidden != "" {
		if value, err := strconv.ParseBool(includeHidden); err == nil {
			opts.IncludeHidden = value
		}
	}

	if sortBy := r.URL.Query().Get("sortBy"); sortBy != "" {
		// Validate sort field
		validSortFields := map[string]bool{"name": true, "size": true, "modTime": true, "type": true}
		if validSortFields[sortBy] {
			opts.SortBy = sortBy
		}
	}

	if sortDir := r.URL.Query().Get("sortDir"); sortDir != "" {
		// Validate sort direction
		if sortDir == "asc" || sortDir == "desc" {
			opts.SortDir = sortDir
		}
	}

	if filter := r.URL.Query().Get("filter"); filter != "" {
		opts.Filter = filter
	}

	return opts
}
