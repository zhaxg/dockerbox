package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jR4dh3y/BoxBox/backend/internal/config"
)

// ComposeProjectRecord represents a BoxBox-created compose project.
type ComposeProjectRecord struct {
	HostID    string `json:"hostId"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	CreatedAt string `json:"createdAt"`
}

// ComposeStore provides persistence for BoxBox-created compose projects.
type ComposeStore struct {
	mu       sync.RWMutex
	projects []ComposeProjectRecord
}

var composeStoreInstance *ComposeStore
var composeStoreOnce sync.Once

// GetComposeStore returns the singleton ComposeStore instance.
func GetComposeStore() *ComposeStore {
	composeStoreOnce.Do(func() {
		composeStoreInstance = &ComposeStore{}
		composeStoreInstance.load()
	})
	return composeStoreInstance
}

// load reads the compose projects file.
func (s *ComposeStore) load() {
	data, err := os.ReadFile(config.ComposeProjectsFile)
	if err != nil {
		s.projects = make([]ComposeProjectRecord, 0)
		return
	}
	var projects []ComposeProjectRecord
	if err := json.Unmarshal(data, &projects); err != nil {
		s.projects = make([]ComposeProjectRecord, 0)
		return
	}
	s.projects = projects
}

// save writes the compose projects file.
func (s *ComposeStore) save() error {
	data, err := json.MarshalIndent(s.projects, "", "  ")
	if err != nil {
		return err
	}
	// Ensure data directory exists
	dir := filepath.Dir(config.ComposeProjectsFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(config.ComposeProjectsFile, data, 0644)
}

// Add adds a new compose project record.
func (s *ComposeStore) Add(hostID, name, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record := ComposeProjectRecord{
		HostID:    hostID,
		Name:      name,
		Path:      path,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.projects = append(s.projects, record)
	return s.save()
}

// Remove removes compose project records by host ID and name.
func (s *ComposeStore) Remove(hostID, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := make([]ComposeProjectRecord, 0, len(s.projects))
	for _, p := range s.projects {
		if !(p.HostID == hostID && p.Name == name) {
			filtered = append(filtered, p)
		}
	}
	s.projects = filtered
	return s.save()
}

// ListByHost returns all compose project records for a given host.
func (s *ComposeStore) ListByHost(hostID string) []ComposeProjectRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []ComposeProjectRecord
	for _, p := range s.projects {
		if p.HostID == hostID {
			result = append(result, p)
		}
	}
	return result
}

// ListAll returns all compose project records.
func (s *ComposeStore) ListAll() []ComposeProjectRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ComposeProjectRecord, len(s.projects))
	copy(result, s.projects)
	return result
}
