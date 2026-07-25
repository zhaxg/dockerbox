package service

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HostStats represents a single point-in-time host stats snapshot.
type HostStats struct {
	Timestamp int64   `json:"ts"`
	CPU       float64 `json:"cpu"`
	CPUCores  int     `json:"cpuCores"`
	MemTotal  int64   `json:"memTotal"`
	MemUsed   int64   `json:"memUsed"`
	MemPct    float64 `json:"memPct"`
	NetRx     uint64  `json:"netRx"`
	NetTx     uint64  `json:"netTx"`
	Load1     float64 `json:"load1"`
	Load5     float64 `json:"load5"`
	Load15    float64 `json:"load15"`
}

// DockerStatsSnapshot represents Docker-level stats.
type DockerStatsSnapshot struct {
	ContainersTotal   int   `json:"containersTotal"`
	ContainersRunning int   `json:"containersRunning"`
	ContainersStopped int   `json:"containersStopped"`
	ComposeTotal      int   `json:"composeTotal"`
	ComposeRunning    int   `json:"composeRunning"`
	ImagesTotal       int   `json:"imagesTotal"`
	ImagesSize        int64 `json:"imagesSize"`
}

// StatsSnapshot is the full overview payload.
type StatsSnapshot struct {
	Host   HostStats         `json:"host"`
	Docker DockerStatsSnapshot `json:"docker"`
}

// CollectorBackground continuously collects host + Docker stats.
// FileReader reads files from a host (local or remote via SSH).
type FileReader interface {
	ReadFile(path string) ([]byte, error)
}

// localFileReader reads from the local filesystem.
type LocalFileReader struct{}

func (r *LocalFileReader) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// sshFileReader reads files from a remote host via SSH.
type SSHFileReader struct {
	Docker *DockerService
}

func (r *SSHFileReader) ReadFile(path string) ([]byte, error) {
	out, err := r.Docker.SSHExec(context.Background(), "cat "+path)
	if err != nil {
			return nil, err
	}
	return []byte(out), nil
}

type CollectorBackground struct {
	docker       *DockerService
	reader       FileReader // local or SSH

	mu       sync.RWMutex
	latest   StatsSnapshot
	history  []HostStats // ring buffer for 1h @ 2s = 1800 points
	maxPoints int

	stopCh chan struct{}
}

const (
	collectInterval  = 2 * time.Second
	maxHistoryPoints = 1800 // 1 hour at 2s interval
)

// NewCollector creates and starts a background stats collector.
func NewCollector(ctx context.Context, docker *DockerService) *CollectorBackground {
	return NewCollectorWithReader(ctx, docker, &LocalFileReader{})
}

func NewCollectorWithReader(ctx context.Context, docker *DockerService, reader FileReader) *CollectorBackground {
	c := &CollectorBackground{
		docker:       docker,
		reader:       reader,
		history:      make([]HostStats, 0, maxHistoryPoints),
		maxPoints:    maxHistoryPoints,
		stopCh:       make(chan struct{}),
	}
	go c.run(ctx)
	return c
}

// Stop terminates the collector goroutine.
func (c *CollectorBackground) Stop() {
	close(c.stopCh)
}

// Latest returns the most recent snapshot.
func (c *CollectorBackground) Latest() StatsSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latest
}

// History returns the full history buffer (up to 1h).
func (c *CollectorBackground) History() []HostStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]HostStats, len(c.history))
	copy(out, c.history)
	return out
}

func (c *CollectorBackground) run(ctx context.Context) {
	// Collect immediately on start
	c.collect()

	ticker := time.NewTicker(collectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.collect()
		}
	}
}

func (c *CollectorBackground) collect() {
	snap := StatsSnapshot{}

	// Host stats from /proc
	snap.Host = c.readHostStats()

	// Docker stats
	snap.Docker = c.readDockerStats()

	c.mu.Lock()
	c.latest = snap
	c.history = append(c.history, snap.Host)
	if len(c.history) > c.maxPoints {
		c.history = c.history[len(c.history)-c.maxPoints:]
	}
	c.mu.Unlock()
}

func (c *CollectorBackground) readHostStats() HostStats {
	h := HostStats{Timestamp: time.Now().UnixMilli()}
	r := c.reader

	// Try multiple paths; remote hosts only have /proc/
	procPaths := []string{"/proc", "/host_root/proc", "/host/proc"}

	for _, base := range procPaths {
		if data, err := r.ReadFile(base + "/stat"); err == nil {
			h.parseCPU(string(data))
			break
		}
	}
	for _, base := range procPaths {
		if data, err := r.ReadFile(base + "/meminfo"); err == nil {
			h.parseMem(string(data))
			break
		}
	}
	for _, base := range procPaths {
		if data, err := r.ReadFile(base + "/net/dev"); err == nil {
			h.parseNet(string(data))
			break
		}
	}
	for _, base := range procPaths {
		if data, err := r.ReadFile(base + "/loadavg"); err == nil {
			h.parseLoad(string(data))
			break
		}
	}

	return h
}

func (h *HostStats) parseCPU(raw string) {
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 5 {
				continue
			}
			var total uint64
			for _, f := range fields[1:] {
				v, _ := strconv.ParseUint(f, 10, 64)
				total += v
			}
			idle, _ := strconv.ParseUint(fields[4], 10, 64)
			if total > 0 {
				h.CPU = float64((total-idle)*100) / float64(total)
			}
			break
		}
	}
	// Count CPU cores (lines starting with "cpuN")
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "cpu") && len(line) > 3 && line[3] >= '0' && line[3] <= '9' {
			h.CPUCores++
		}
	}
}

func (h *HostStats) parseMem(raw string) {
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			h.MemTotal = parseKBField(line) * 1024
		} else if strings.HasPrefix(line, "MemAvailable:") {
			avail := parseKBField(line) * 1024
			if h.MemTotal > 0 {
				h.MemUsed = h.MemTotal - avail
				h.MemPct = float64(h.MemUsed*100) / float64(h.MemTotal)
			}
		}
	}
}

func (h *HostStats) parseNet(raw string) {
	for _, line := range strings.Split(raw, "\n") {
		if strings.Contains(line, ":") && !strings.HasPrefix(line, "Inter") && !strings.HasPrefix(line, "face") {
			fields := strings.Fields(strings.TrimLeft(line, " "))
			// fields[0] = "intf:", fields[1..8] = RX, fields[9..] = TX
			if len(fields) >= 10 {
				rx, _ := strconv.ParseUint(fields[1], 10, 64)
				tx, _ := strconv.ParseUint(fields[9], 10, 64)
				h.NetRx += rx
				h.NetTx += tx
			}
		}
	}
}

func (h *HostStats) parseLoad(raw string) {
	trimmed := strings.TrimSpace(raw)
	fields := strings.Fields(trimmed)
	if len(fields) >= 3 {
		h.Load1, _ = strconv.ParseFloat(fields[0], 64)
		h.Load5, _ = strconv.ParseFloat(fields[1], 64)
		h.Load15, _ = strconv.ParseFloat(fields[2], 64)
	}
}

func parseKBField(line string) int64 {
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		v, _ := strconv.ParseInt(fields[1], 10, 64)
		return v
	}
	return 0
}

func (c *CollectorBackground) readDockerStats() DockerStatsSnapshot {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	snap := DockerStatsSnapshot{}

	if stats, err := c.docker.GetDockerStats(ctx); err == nil {
		snap.ContainersTotal = stats.Containers.Total
		snap.ContainersRunning = stats.Containers.Running
		snap.ContainersStopped = stats.Containers.Stopped
		snap.ImagesTotal = stats.Images.Total
		snap.ImagesSize = stats.Images.Size
	}

	// Count compose projects separately
	projects, err := c.docker.ListComposeProjects(ctx)
	if err == nil {
		snap.ComposeTotal = len(projects)
		for _, p := range projects {
			if p.Status == "running" {
				snap.ComposeRunning++
			}
		}
	}

	return snap
}
