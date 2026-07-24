package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jR4dh3y/BoxBox/backend/internal/model"
	"github.com/jR4dh3y/BoxBox/backend/internal/service"
)

// JobHandler handles job-related HTTP requests
type JobHandler struct {
	jobService      service.JobService
	hostAccess      map[string]service.HostFileAccess
	hostMountPoints map[string]map[string]string // hostID -> mountName -> hostPath
}

// NewJobHandler creates a new job handler
func NewJobHandler(jobService service.JobService) *JobHandler {
	return &JobHandler{
		jobService:      jobService,
		hostAccess:      make(map[string]service.HostFileAccess),
		hostMountPoints: make(map[string]map[string]string),
	}
}

// SetHostAccess registers a HostFileAccess implementation for a host ID.
func (h *JobHandler) SetHostAccess(hostID string, access service.HostFileAccess) {
	h.hostAccess[hostID] = access
}

// SetHostMountPoints sets mount point paths for a host.
func (h *JobHandler) SetHostMountPoints(hostID string, mounts map[string]string) {
	h.hostMountPoints[hostID] = mounts
}

func (h *JobHandler) getHostAccess(r *http.Request) service.HostFileAccess {
	hostID := r.Header.Get("X-Host-ID")
	if hostID != "" {
		if access, ok := h.hostAccess[hostID]; ok {
			return access
		}
	}
	return nil
}

func (h *JobHandler) resolvePath(r *http.Request, boxPath string) string {
	hostID := r.Header.Get("X-Host-ID")
	parts := strings.SplitN(boxPath, "/", 2)
	mountName := parts[0]
	subPath := ""
	if len(parts) > 1 {
		subPath = parts[1]
	}
	if mounts, ok := h.hostMountPoints[hostID]; ok {
		if hostPath, ok := mounts[mountName]; ok {
			if subPath != "" {
				return hostPath + "/" + subPath
			}
			return hostPath
		}
	}
	return "/" + boxPath
}

func (h *JobHandler) getHostID(r *http.Request) string {
	return r.Header.Get("X-Host-ID")
}

// RegisterRoutes registers job routes on the given router
func (h *JobHandler) RegisterRoutes(r chi.Router) {
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/{id}", h.Get)
	r.Delete("/{id}", h.Cancel)
}

// CreateJobRequest represents the create job request body
type CreateJobRequest struct {
	Type       string `json:"type"`
	SourcePath string `json:"sourcePath"`
	DestPath   string `json:"destPath,omitempty"`
}

// JobResponse represents a job in API responses
type JobResponse struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	State       string `json:"state"`
	Progress    int    `json:"progress"`
	SourcePath  string `json:"sourcePath"`
	DestPath    string `json:"destPath,omitempty"`
	Error       string `json:"error,omitempty"`
	CreatedAt   string `json:"createdAt"`
	StartedAt   string `json:"startedAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
}

// JobListResponse represents the list of jobs
type JobListResponse struct {
	Jobs []JobResponse `json:"jobs"`
}

// List returns all jobs
func (h *JobHandler) List(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.jobService.List(r.Context())
	if err != nil {
		writeError(w, "Failed to list jobs", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	response := JobListResponse{
		Jobs: make([]JobResponse, len(jobs)),
	}

	for i, job := range jobs {
		response.Jobs[i] = h.toJobResponse(job)
	}

	writeJSON(w, response, http.StatusOK)
}

// Get returns a job by ID
func (h *JobHandler) Get(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	if jobID == "" {
		writeError(w, "Job ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	job, err := h.jobService.Get(r.Context(), jobID)
	if err != nil {
		HandleServiceError(w, err)
		return
	}

	writeJSON(w, h.toJobResponse(job), http.StatusOK)
}

// Create creates a new job
func (h *JobHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	jobType := model.JobType(req.Type)
	if !jobType.IsValid() {
		writeError(w, "Invalid job type. Must be 'copy', 'move', or 'delete'", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if req.SourcePath == "" {
		writeError(w, "Source path is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if (jobType == model.JobTypeCopy || jobType == model.JobTypeMove) && req.DestPath == "" {
		writeError(w, "Destination path is required for copy and move operations", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	// Remote/socket host: execute synchronously via HostFileAccess
	if access := h.getHostAccess(r); access != nil {
		job := &model.Job{
			Type:       jobType,
			State:      model.JobStateRunning,
			SourcePath: req.SourcePath,
			DestPath:   req.DestPath,
		}

		srcPath := h.resolvePath(r, req.SourcePath)
		var dstPath string
		if req.DestPath != "" {
			dstPath = h.resolvePath(r, req.DestPath)
		}

		var execErr error
		switch jobType {
		case model.JobTypeCopy:
			execErr = h.remoteCopy(access, r, srcPath, dstPath)
		case model.JobTypeMove:
			execErr = h.remoteMove(access, r, srcPath, dstPath)
		case model.JobTypeDelete:
			execErr = access.Remove(r.Context(), srcPath)
		}

		if execErr != nil {
			job.State = model.JobStateFailed
			job.Error = execErr.Error()
		} else {
			job.State = model.JobStateCompleted
			job.Progress = 100
		}

		writeJSON(w, h.toJobResponse(job), http.StatusAccepted)
		return
	}

	// Local: queue as background job
	params := model.JobParams{
		Type:       jobType,
		SourcePath: req.SourcePath,
		DestPath:   req.DestPath,
	}

	job, err := h.jobService.Create(r.Context(), params)
	if err != nil {
		HandleServiceError(w, err)
		return
	}

	writeJSON(w, h.toJobResponse(job), http.StatusAccepted)
}

// Cancel cancels a running job
func (h *JobHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	if jobID == "" {
		writeError(w, "Job ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if err := h.jobService.Cancel(r.Context(), jobID); err != nil {
		HandleServiceError(w, err)
		return
	}

	writeJSON(w, map[string]string{"message": "Job cancelled successfully"}, http.StatusOK)
}

// remoteCopy copies a file via HostFileAccess
func (h *JobHandler) remoteCopy(access service.HostFileAccess, r *http.Request, src, dst string) error {
	data, err := access.Read(r.Context(), src)
	if err != nil {
		return err
	}
	return access.Write(r.Context(), dst, data)
}

// remoteMove moves a file via HostFileAccess (rename or copy+delete)
func (h *JobHandler) remoteMove(access service.HostFileAccess, r *http.Request, src, dst string) error {
	if err := access.Rename(r.Context(), src, dst); err != nil {
		// Fallback: copy + delete
		data, readErr := access.Read(r.Context(), src)
		if readErr != nil {
			return readErr
		}
		if writeErr := access.Write(r.Context(), dst, data); writeErr != nil {
			return writeErr
		}
		return access.Remove(r.Context(), src)
	}
	return nil
}

// toJobResponse converts a model.Job to JobResponse
func (h *JobHandler) toJobResponse(job *model.Job) JobResponse {
	resp := JobResponse{
		ID:         job.ID,
		Type:       string(job.Type),
		State:      string(job.State),
		Progress:   job.Progress,
		SourcePath: job.SourcePath,
		DestPath:   job.DestPath,
		Error:      job.Error,
		CreatedAt:  job.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if !job.StartedAt.IsZero() {
		resp.StartedAt = job.StartedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	if !job.CompletedAt.IsZero() {
		resp.CompletedAt = job.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	return resp
}
