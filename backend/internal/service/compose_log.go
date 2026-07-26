package service

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ComposeLogManager manages per-project compose log files.
type ComposeLogManager struct {
	dir    string
	file   *os.File
	writer *bufio.Writer
	mu     sync.Mutex
}

var composeLogDir = "./logs"

// LogDir returns the compose log directory.
func LogDir() string {
	return composeLogDir
}

// InitLogDir ensures the logs directory exists.
func InitLogDir() error {
	return os.MkdirAll(composeLogDir, 0755)
}

// OpenLog opens (or creates) the log file for a compose project on a specific host.
func OpenLog(hostID, projectName string) (*ComposeLogManager, error) {
	dir := filepath.Join(composeLogDir, hostID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log dir: %w", err)
	}
	path := logPath(hostID, projectName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	return &ComposeLogManager{
		dir:    dir,
		file:   f,
		writer: bufio.NewWriter(f),
	}, nil
}

// WriteLine appends a timestamped line to the log file.
func (m *ComposeLogManager) WriteLine(line string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writer == nil {
		return
	}
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(m.writer, "[%s] %s\n", ts, line)
	m.writer.Flush()
}

// Close flushes and closes the log file.
func (m *ComposeLogManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writer != nil {
		m.writer.Flush()
	}
	if m.file != nil {
		m.file.Close()
		m.file = nil
		m.writer = nil
	}
}

// DeleteLog removes the log file for a compose project on a specific host.
func DeleteLog(hostID, projectName string) {
	path := logPath(hostID, projectName)
	os.Remove(path)
}

// ReadLastLines reads the last n lines from the compose log file.
func ReadLastLines(hostID, projectName string, n int) []string {
	path := logPath(hostID, projectName)
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if n <= 0 || n >= len(lines) {
		return lines
	}
	return lines[len(lines)-n:]
}

func logPath(hostID, projectName string) string {
	// Sanitize: replace path separators with underscores
	safe := strings.ReplaceAll(projectName, "/", "_")
	safe = strings.ReplaceAll(safe, "\\", "_")
	return filepath.Join(composeLogDir, hostID, "compose-"+safe+".log")
}
