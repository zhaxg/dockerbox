// Package handler provides HTTP handlers for the BoxBox API.
package handler

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jR4dh3y/BoxBox/backend/internal/service"
)

// SSEHandler handles Server-Sent Events connections.
type SSEHandler struct {
	dockerService *service.DockerService
}

// NewSSEHandler creates a new SSE handler.
func NewSSEHandler(dockerService *service.DockerService) *SSEHandler {
	return &SSEHandler{dockerService: dockerService}
}

// RegisterRoutes registers SSE routes on the given router.
func (h *SSEHandler) RegisterRoutes(r chi.Router) {
	r.Get("/stats", h.StreamStats)
	r.Get("/logs/{id}", h.StreamLogs)
	r.Get("/host", h.StreamHostStats)
}

// writeSSEHeaders sets the standard SSE headers on the response.
func writeSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}

// StreamStats streams container stats via SSE.
func (h *SSEHandler) StreamStats(w http.ResponseWriter, r *http.Request) {
	writeSSEHeaders(w)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			stats, err := h.dockerService.GetDockerStats(r.Context())
			if err != nil {
				fmt.Fprintf(w, "event: error\ndata: {\"error\":\"%s\"}\n\n", escapeJSON(err.Error()))
				flusher.Flush()
				continue
			}
			fmt.Fprintf(w, "event: stats\ndata: {\"containers\":%d,\"running\":%d,\"stopped\":%d,\"images\":%d}\n\n",
				stats.Containers.Total, stats.Containers.Running, stats.Containers.Stopped, stats.Images.Total)
			flusher.Flush()
		}
	}
}

// StreamLogs streams container logs via SSE.
func (h *SSEHandler) StreamLogs(w http.ResponseWriter, r *http.Request) {
	writeSSEHeaders(w)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "Container ID required", http.StatusBadRequest)
		return
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastLen int

	fmt.Fprintf(w, "event: connected\ndata: {\"container\":\"%s\"}\n\n", escapeJSON(id))
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			logs, err := h.dockerService.GetContainerLogs(r.Context(), id, 50)
			if err != nil {
				fmt.Fprintf(w, "event: error\ndata: {\"error\":\"%s\"}\n\n", escapeJSON(err.Error()))
				flusher.Flush()
				continue
			}
			if len(logs) > lastLen {
				for _, line := range logs[lastLen:] {
					if line != "" {
						fmt.Fprintf(w, "event: log\ndata: %s\n\n", escapeJSON(line))
					}
				}
				flusher.Flush()
				lastLen = len(logs)
			}
		}
	}
}

// StreamHostStats streams host system stats via SSE.
// detectProcBase finds the host proc filesystem path.
func detectProcBase() string {
	for _, p := range []string{"/host_root/proc", "/host/proc", "/proc"} {
		if _, err := os.Stat(p + "/stat"); err == nil {
			return p
		}
	}
	return "/proc"
}

func (h *SSEHandler) StreamHostStats(w http.ResponseWriter, r *http.Request) {
	writeSSEHeaders(w)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	procBase := detectProcBase()

	fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			cpu := readFile(procBase + "/stat")
			mem := readFile(procBase + "/meminfo")
			net := readFile(procBase + "/net/dev")
			load := readFile(procBase + "/loadavg")

			data := fmt.Sprintf(`{"cpu":%s,"memory":%s,"network":%s,"load":%s}`,
				wrapRaw(cpu), wrapRaw(mem), wrapRaw(net), wrapRaw(load))

			fmt.Fprintf(w, "event: host\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf(`{"error":"%s"}`, escapeJSON(err.Error()))
	}
	return fmt.Sprintf(`{"raw":"%s"}`, escapeJSON(string(data)))
}

func wrapRaw(content string) string {
	return content
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}
