package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"dockerbox/backend/internal/service"
)

// SSEHandler handles Server-Sent Events connections.
type SSEHandler struct {
	dockerService    *service.DockerService
	collector        *service.CollectorBackground // default collector
	services         map[string]*service.DockerService
	hostCollectors   map[string]*service.CollectorBackground
	defaultHostID    string
}

// NewSSEHandler creates a new SSE handler.
func NewSSEHandler(dockerService *service.DockerService, collector *service.CollectorBackground) *SSEHandler {
	return &SSEHandler{
		dockerService:  dockerService,
		collector:      collector,
		services:       make(map[string]*service.DockerService),
		hostCollectors: make(map[string]*service.CollectorBackground),
	}
}

// SetService registers a DockerService for a host ID.
func (h *SSEHandler) SetService(hostID string, svc *service.DockerService) {
	h.services[hostID] = svc
}

// SetCollector registers a CollectorBackground for a host ID.
func (h *SSEHandler) SetCollector(hostID string, c *service.CollectorBackground) {
	h.hostCollectors[hostID] = c
}

// SetDefaultHost sets the default host ID.
func (h *SSEHandler) SetDefaultHost(hostID string) {
	h.defaultHostID = hostID
}

// getCollector returns the collector for the current request's host.
func (h *SSEHandler) getCollector(r *http.Request) *service.CollectorBackground {
	hostID := getHostID(r)
	if hostID == "" {
		hostID = r.URL.Query().Get("host")
	}
	if hostID == "" {
		hostID = r.URL.Query().Get("host")
	}
	if hostID != "" {
		if c, ok := h.hostCollectors[hostID]; ok {
			return c
		}
	}
	if h.defaultHostID != "" {
		if c, ok := h.hostCollectors[h.defaultHostID]; ok {
			return c
		}
	}
	return h.collector
}

// getDockerService returns the DockerService for the current request's host.
func (h *SSEHandler) getDockerService(r *http.Request) *service.DockerService {
	hostID := getHostID(r)
	if hostID == "" {
		hostID = r.URL.Query().Get("host")
	}
	if hostID == "" {
		hostID = r.URL.Query().Get("host")
	}
	if hostID != "" {
		if svc, ok := h.services[hostID]; ok {
			return svc
		}
	}
	if h.defaultHostID != "" {
		if svc, ok := h.services[h.defaultHostID]; ok {
			return svc
		}
	}
	return h.dockerService
}

// RegisterRoutes registers SSE routes on the given router.
func (h *SSEHandler) RegisterRoutes(r chi.Router) {
	r.Get("/stats", h.StreamStats)
	r.Get("/logs/{id}", h.StreamLogs)
	r.Get("/host", h.StreamHostStats)
	r.Get("/overview", h.GetOverview)
}

// GetOverview returns the latest snapshot + history for the overview page.
func (h *SSEHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
	c := h.getCollector(r)
	if c == nil {
		writeJSON(w, map[string]interface{}{"host": service.HostStats{}, "docker": service.DockerStatsSnapshot{}, "history": []service.HostStats{}}, http.StatusOK)
		return
	}
	latest := c.Latest()
	history := c.History()
	writeJSON(w, map[string]interface{}{
		"host":    latest.Host,
		"docker":  latest.Docker,
		"history": history,
	}, http.StatusOK)
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

	docker := h.getDockerService(r)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			stats, err := docker.GetDockerStats(r.Context())
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

	docker := h.getDockerService(r)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Track last seen line content to detect new logs
	lastLines := make(map[string]bool)

	fmt.Fprintf(w, "event: connected\ndata: {\"container\":\"%s\"}\n\n", escapeJSON(id))
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			logs, err := docker.GetContainerLogs(r.Context(), id, 200)
			if err != nil {
				continue
			}
			for _, line := range logs {
				if line == "" {
					continue
				}
				if !lastLines[line] {
					lastLines[line] = true
					fmt.Fprintf(w, "event: log\ndata: %s\n\n", escapeJSON(line))
				}
			}
			flusher.Flush()
			// Cleanup old entries if map grows too large
			if len(lastLines) > 1000 {
				lastLines = make(map[string]bool)
				for _, line := range logs {
					if line != "" {
						lastLines[line] = true
					}
				}
			}
		}
	}
}

// StreamHostStats streams host stats from the collector via SSE.
func (h *SSEHandler) StreamHostStats(w http.ResponseWriter, r *http.Request) {
	writeSSEHeaders(w)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	c := h.getCollector(r)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if c == nil {
				continue
			}
			snap := c.Latest()
			data, _ := json.Marshal(snap.Host)
			fmt.Fprintf(w, "event: host\ndata: %s\n\n", string(data))
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

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}
