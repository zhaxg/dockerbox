package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"dockerbox/backend/internal/service"
)

// StreamComposeLogs streams compose logs via SSE.
func (h *DockerHandler) StreamComposeLogs(w http.ResponseWriter, r *http.Request) {
	writeSSEHeaders(w)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "Project ID required", http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), r, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	runner := service.GetComposeRunner()

	// Check if there's an active compose operation for this project
	// If so, stream its output (shows pull/build progress)
	if activeRun := runner.Get(id); activeRun != nil && activeRun.GetStatus() == service.ComposeRunning {
		fmt.Fprintf(w, "event: connected\ndata: {\"id\":\"%s\"}\n\n", escapeJSON(id))
		flusher.Flush()

		lastIdx := 0
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-activeRun.Done:
				output := activeRun.GetOutput()
				for i := lastIdx; i < len(output); i++ {
					if output[i] != "" {
						data, _ := json.Marshal(map[string]string{"line": output[i]})
						fmt.Fprintf(w, "event: log\ndata: %s\n\n", string(data))
					}
				}
				flusher.Flush()
				status := string(activeRun.GetStatus())
				data, _ := json.Marshal(map[string]string{"status": status})
				fmt.Fprintf(w, "event: done\ndata: %s\n\n", string(data))
				flusher.Flush()
				return
			case <-ticker.C:
				output := activeRun.GetOutput()
				for i := lastIdx; i < len(output); i++ {
					if output[i] != "" {
						lineData, _ := json.Marshal(map[string]string{"line": output[i]})
						fmt.Fprintf(w, "event: log\ndata: %s\n\n", string(lineData))
					}
				}
				if len(output) > lastIdx {
					flusher.Flush()
					lastIdx = len(output)
				}
			}
		}
	}

	// No active operation — fall back to container logs
	logID := "logs-" + id
	run := runner.StartLogs(logID, getHostID(r), h.getService(r).GetSSHHost(), h.getService(r).GetSSHKey(), h.getService(r).Runtime(), path)

	fmt.Fprintf(w, "event: connected\ndata: {\"id\":\"%s\"}\n\n", escapeJSON(id))
	flusher.Flush()

	lastIdx := 0
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			run.Abort()
			return
		case <-run.Done:
			output := run.GetOutput()
			for i := lastIdx; i < len(output); i++ {
				if output[i] != "" {
					data, _ := json.Marshal(map[string]string{"line": output[i]})
					fmt.Fprintf(w, "event: log\ndata: %s\n\n", string(data))
				}
			}
			flusher.Flush()
			status := string(run.GetStatus())
			data, _ := json.Marshal(map[string]string{"status": status})
			fmt.Fprintf(w, "event: done\ndata: %s\n\n", string(data))
			flusher.Flush()
			return
		case <-ticker.C:
			output := run.GetOutput()
			for i := lastIdx; i < len(output); i++ {
				if output[i] != "" {
					lineData, _ := json.Marshal(map[string]string{"line": output[i]})
					fmt.Fprintf(w, "event: log\ndata: %s\n\n", string(lineData))
				}
			}
			if len(output) > lastIdx {
				flusher.Flush()
				lastIdx = len(output)
			}
		}
	}
}


// AbortComposeOperation kills a running compose operation.
func (h *DockerHandler) AbortComposeOperation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID required", "VALIDATION_ERROR", http.StatusBadRequest)
		return
	}

	runner := service.GetComposeRunner()
	run := runner.Get(id)
	if run == nil {
		writeError(w, "No active operation found", "NOT_FOUND", http.StatusNotFound)
		return
	}

	run.Abort()
	writeJSON(w, map[string]string{"message": "Operation aborted"}, http.StatusOK)
}

// GetComposeStatus returns the status of a compose operation.

