// Package handler provides HTTP handlers for Docker Compose operations.
package handler

import (
	"fmt"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jR4dh3y/BoxBox/backend/internal/model"
	"github.com/jR4dh3y/BoxBox/backend/internal/service"
)

func (h *DockerHandler) ListComposeProjects(w http.ResponseWriter, r *http.Request) {
	hostID := getHostID(r)
	if hostID == "" {
		hostID = h.defaultHostID
	}

	projects, err := h.getService(r).ListComposeProjects(r.Context())
	if err != nil {
		writeError(w, "Failed to list compose projects", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	// Add hostId to container-discovered projects
	for i := range projects {
		projects[i].HostID = hostID
	}

	// Merge with BoxBox-created projects from store
	seen := make(map[string]bool)
	for _, p := range projects {
		seen[p.Name] = true
	}

	store := service.GetComposeStore()
	storeProjects := store.ListByHost(hostID)
	for _, sp := range storeProjects {
		if seen[sp.Name] {
			continue
		}
		projects = append(projects, model.ComposeProject{
			HostID: hostID,
			ID:     sp.Name,
			Name:   sp.Name,
			Path:   sp.Path,
			Status: "stopped",
		})
		seen[sp.Name] = true
	}

	writeJSON(w, map[string]interface{}{"projects": projects}, http.StatusOK)
}

// CreateComposeProject creates a new compose project.
func (h *DockerHandler) CreateComposeProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name            string `json:"name"`
		ComposeContent  string `json:"composeContent"`
		EnvContent      string `json:"envContent"`
		BasePath        string `json:"basePath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		writeError(w, "Project name is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if req.ComposeContent == "" {
		writeError(w, "Compose content is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	// Check for duplicate project name
	hostID := getHostID(r)
	if hostID == "" {
		hostID = h.defaultHostID
	}
	store := service.GetComposeStore()
	for _, p := range store.ListByHost(hostID) {
		if p.Name == req.Name {
			writeError(w, "项目名称已存在", model.ErrCodeValidationError, http.StatusConflict)
			return
		}
	}

	// Default to first compose path for current host
	if req.BasePath == "" {
		paths := h.getComposePaths(r)
		if len(paths) > 0 {
			req.BasePath = paths[0]
		} else {
			req.BasePath = "/vol1/1000/docker"
		}
	}

	svc := h.getService(r)
	result, err := svc.CreateComposeProject(r.Context(), req.Name, req.ComposeContent, req.EnvContent, req.BasePath)
	if err != nil {
		writeError(w, result.Message, model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	// Persist to compose store
	store.Add(hostID, req.Name, req.BasePath+"/"+req.Name)

	writeJSON(w, result, http.StatusOK)
}

// ComposeUp starts/restarts a compose project based on action type.
func (h *DockerHandler) ComposeUp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), r, id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	// Detect actual container state, pick the right command
	svc := h.getService(r)
	args, msg := svc.GetComposeUpArgs(r.Context(), id)

	// Log the action
	if lm, err := service.OpenLog(id); err == nil {
		action := "up"
		if len(args) == 1 && args[0] == "start" {
			action = "start"
		} else if len(args) >= 3 && args[2] == "--force-recreate" {
			action = "recreate"
		}
		lm.WriteLine(fmt.Sprintf("=== Compose %s ===", action))
		lm.Close()
	}

	runner := service.GetComposeRunner()
	runner.Start(id, svc.GetSSHHost(), svc.GetSSHKey(), svc.Runtime(), args, path)

	// action: "start" = existing containers, "up" = need pull/create, "recreate" = force recreate
	action := "up"
	if len(args) == 1 && args[0] == "start" {
		action = "start"
	} else if len(args) >= 3 && args[2] == "--force-recreate" {
		action = "recreate"
	}

	writeJSON(w, map[string]string{"status": "started", "message": msg, "action": action}, http.StatusOK)
}

// ComposeDown stops a compose project (docker compose stop).
func (h *DockerHandler) ComposeDown(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), r, id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	// Synchronous stop — avoids runner race with concurrent up
	svc := h.getService(r)

	// Log the action
	if lm, err := service.OpenLog(id); err == nil {
		lm.WriteLine("=== Compose Down ===")
		lm.Close()
	}

	result, err := svc.ComposeDown(r.Context(), path)
	if err != nil {
		if lm, err2 := service.OpenLog(id); err2 == nil {
			lm.WriteLine(fmt.Sprintf("Down failed: %v", err))
			lm.Close()
		}
		writeError(w, result.Message, model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	if lm, err2 := service.OpenLog(id); err2 == nil {
		lm.WriteLine("Compose stopped successfully")
		lm.Close()
	}

	writeJSON(w, map[string]string{"status": "stopped", "message": "Compose stopped"}, http.StatusOK)
}

// ComposeBuild builds a compose project.
func (h *DockerHandler) ComposeBuild(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), r, id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	result, err := h.getService(r).ComposeBuild(r.Context(), path)
	if err != nil {
		writeError(w, result.Message, model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, result, http.StatusOK)
}

// ComposeRestart restarts a compose project.
func (h *DockerHandler) ComposeRestart(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), r, id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	// Log the action
	if lm, err := service.OpenLog(id); err == nil {
		lm.WriteLine("=== Compose Restart ===")
		lm.Close()
	}

	runner := service.GetComposeRunner()
	runner.Start(id, h.getService(r).GetSSHHost(), h.getService(r).GetSSHKey(), h.getService(r).Runtime(), []string{"restart"}, path)

	writeJSON(w, map[string]string{"status": "started", "message": "Compose restart started"}, http.StatusOK)
}

// ComposePull pulls images for a compose project.
func (h *DockerHandler) ComposePull(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), r, id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	result, err := h.getService(r).ComposePull(r.Context(), path)
	if err != nil {
		writeError(w, result.Message, model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, result, http.StatusOK)
}

// ComposeRedeploy runs docker-compose down then up.
func (h *DockerHandler) ComposeRedeploy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), r, id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	runner := service.GetComposeRunner()
	runner.StartRedeploy(id, h.getService(r).GetSSHHost(), h.getService(r).GetSSHKey(), h.getService(r).Runtime(), path)

	writeJSON(w, map[string]string{"status": "started", "message": "Redeploy started"}, http.StatusOK)
}

// ComposeRebuild rebuilds a compose project (docker compose up -d --build).
func (h *DockerHandler) ComposeRebuild(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), r, id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	runner := service.GetComposeRunner()
	runner.Start(id, h.getService(r).GetSSHHost(), h.getService(r).GetSSHKey(), h.getService(r).Runtime(), []string{"up", "-d", "--build"}, path)

	writeJSON(w, map[string]string{"status": "started", "message": "Compose rebuild started"}, http.StatusOK)
}

// ComposeLogs returns compose project logs from the log file.
func (h *DockerHandler) ComposeLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	tail := 100
	if t := r.URL.Query().Get("tail"); t != "" {
		if parsed, err := strconv.Atoi(t); err == nil {
			tail = parsed
		}
	}

	logs := service.ReadLastLines(id, tail)
	writeJSON(w, map[string]interface{}{"lines": logs}, http.StatusOK)
}

// GetComposeFile returns the docker-compose.yml content.
func (h *DockerHandler) GetComposeFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), r, id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	content, err := h.getService(r).GetComposeFile(r.Context(), path)
	if err != nil {
		writeError(w, "Failed to get compose file", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"content": content}, http.StatusOK)
}

// SaveComposeFile saves the docker-compose.yml content.
func (h *DockerHandler) SaveComposeFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), r, id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if err := h.getService(r).SaveComposeFile(path, req.Content); err != nil {
		writeError(w, "Failed to save compose file", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Compose file saved"}, http.StatusOK)
}

// GetComposeEnv returns the .env content.
func (h *DockerHandler) GetComposeEnv(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), r, id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	content, err := h.getService(r).GetComposeEnv(r.Context(), path)
	if err != nil {
		writeError(w, "Failed to get env file", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"content": content}, http.StatusOK)
}

// SaveComposeEnv saves the .env content.
func (h *DockerHandler) SaveComposeEnv(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), r, id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if err := h.getService(r).SaveComposeEnv(path, req.Content); err != nil {
		writeError(w, "Failed to save env file", model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"message": "Env file saved"}, http.StatusOK)
}

// DeleteComposeProject removes a compose project from management (keeps disk files).
func (h *DockerHandler) DeleteComposeProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), r, id)
	if err != nil {
		// If project not found in store/labels, just remove from store
		hostID := getHostID(r)
		if hostID == "" {
			hostID = h.defaultHostID
		}
		store := service.GetComposeStore()
		store.Remove(hostID, id)
		writeJSON(w, map[string]string{"message": "Project removed"}, http.StatusOK)
		return
	}

	// Run docker compose down to stop and remove containers
	runner := service.GetComposeRunner()
	runner.Start(id, h.getService(r).GetSSHHost(), h.getService(r).GetSSHKey(), h.getService(r).Runtime(), []string{"down", "-v"}, path)

	// Remove from compose store
	hostID := getHostID(r)
	if hostID == "" {
		hostID = h.defaultHostID
	}
	store := service.GetComposeStore()
	store.Remove(hostID, id)

	writeJSON(w, map[string]string{"message": "Project removed"}, http.StatusOK)
}

func (h *DockerHandler) CheckComposeName(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, "name is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	hostID := getHostID(r)
	if hostID == "" {
		hostID = h.defaultHostID
	}

	store := service.GetComposeStore()
	for _, p := range store.ListByHost(hostID) {
		if p.Name == name {
			writeJSON(w, map[string]bool{"exists": true}, http.StatusOK)
			return
		}
	}

	writeJSON(w, map[string]bool{"exists": false}, http.StatusOK)
}

// ScanAvailableProjects scans compose paths for projects not yet in the store.
func (h *DockerHandler) ScanAvailableProjects(w http.ResponseWriter, r *http.Request) {
	hostID := getHostID(r)
	if hostID == "" {
		hostID = h.defaultHostID
	}

	store := service.GetComposeStore()
	existing := store.ListByHost(hostID)
	existingMap := make(map[string]bool)
	for _, p := range existing {
		existingMap[p.Name] = true
	}
	// Also exclude projects discovered from container labels
	svc := h.getService(r)
	if projects, err := svc.ListComposeProjects(r.Context()); err == nil {
		for _, p := range projects {
			existingMap[p.Name] = true
		}
	}

	paths := h.getComposePaths(r)
	sshHost := svc.GetSSHHost()
	var available []model.ComposeProject

	for _, basePath := range paths {
		if sshHost != "" {
			// SSH: one command to find all compose files
			sshKey := svc.GetSSHKey()
			discovered, err := sshScanComposeProjects(sshHost, sshKey, basePath)
			if err != nil {
				continue
			}
			for _, d := range discovered {
				if !existingMap[d.Name] {
					available = append(available, d)
				}
			}
		} else {
			// Local: list first-level subdirectories
			dirEntries, err := os.ReadDir(basePath)
			if err != nil {
				continue
			}
			for _, e := range dirEntries {
				if !e.IsDir() || existingMap[e.Name()] {
					continue
				}
				composePath := filepath.Join(basePath, e.Name())
				found := false
				for _, fname := range []string{"docker-compose.yml", "compose.yml", "docker-compose.yaml", "compose.yaml"} {
					if _, err := os.Stat(filepath.Join(composePath, fname)); err == nil {
						found = true
						break
					}
				}
				if !found {
					continue
				}
				available = append(available, model.ComposeProject{
					ID:   e.Name(),
					Name: e.Name(),
					Path: composePath,
				})
			}
		}
	}

	writeJSON(w, map[string]interface{}{"projects": available}, http.StatusOK)
}

// ImportComposeProjects imports discovered projects into the compose store.
func (h *DockerHandler) ImportComposeProjects(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Names []string `json:"names"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	if len(req.Names) == 0 {
		writeError(w, "No projects specified", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	hostID := getHostID(r)
	if hostID == "" {
		hostID = h.defaultHostID
	}

	store := service.GetComposeStore()
	paths := h.getComposePaths(r)

	var imported []string
	for _, name := range req.Names {
		// Check if already imported
		alreadyImported := false
		for _, sp := range store.ListByHost(hostID) {
			if sp.Name == name {
				alreadyImported = true
				break
			}
		}
		if alreadyImported {
			continue
		}

		// Find the project path
		found := false
		for _, basePath := range paths {
			candidate := filepath.Join(basePath, name)
			// For SSH hosts, skip local file check - just add to store
			svc := h.getService(r)
			if svc.GetSSHHost() != "" {
				store.Add(hostID, name, candidate)
				imported = append(imported, name)
				found = true
				break
			}
			// For local hosts, check if compose file exists
			for _, fname := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
				composeFile := filepath.Join(candidate, fname)
				if _, err := os.Stat(composeFile); err == nil {
					store.Add(hostID, name, candidate)
					imported = append(imported, name)
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}

	writeJSON(w, map[string]interface{}{"imported": imported}, http.StatusOK)
}

// sshListDir lists directories in a remote path via SSH.
func sshListDir(sshHost, sshKey, path string) ([]string, error) {
	host, port := parseSSHHost(sshHost)
	keyFile := service.WriteSSHKeyTemp(sshKey)
	defer os.Remove(keyFile)
	
	cmd := exec.Command("ssh", "-i", keyFile, "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		"-p", port, host, "ls -1 "+path)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n"), nil
}

// sshFileExists checks if a file exists on the remote host via SSH.
func sshFileExists(sshHost, sshKey, path string) bool {
	host, port := parseSSHHost(sshHost)
	keyFile := service.WriteSSHKeyTemp(sshKey)
	defer os.Remove(keyFile)
	
	cmd := exec.Command("ssh", "-i", keyFile, "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		"-p", port, host, "test -f "+path+" && echo ok")
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "ok"
}

// parseSSHHost parses sshHost into host and port.
func parseSSHHost(sshHost string) (host, port string) {
	host = sshHost
	port = "22"
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		if _, err := strconv.Atoi(host[idx+1:]); err == nil {
			port = host[idx+1:]
			host = host[:idx]
		}
	}
	return
}

// sshScanComposeProjects scans a remote directory for compose projects in one SSH call.
func sshScanComposeProjects(sshHost, sshKey, basePath string) ([]model.ComposeProject, error) {
	host, port := parseSSHHost(sshHost)
	keyFile := service.WriteSSHKeyTemp(sshKey)
	defer os.Remove(keyFile)
	
	// One SSH call: find all compose files at depth 2
	cmd := exec.Command("ssh", "-i", keyFile, "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		"-p", port, host,
		"find "+basePath+" -maxdepth 2 -name 'docker-compose.yml' -o -name 'compose.yml' -o -name 'docker-compose.yaml' -o -name 'compose.yaml' 2>/dev/null")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	
	var projects []model.ComposeProject
	seen := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Extract project directory from compose file path
		dir := filepath.Dir(line)
		name := filepath.Base(dir)
		if name == "." || name == basePath || seen[name] {
			continue
		}
		seen[name] = true
		projects = append(projects, model.ComposeProject{
			ID:   name,
			Name: name,
			Path: dir,
		})
	}
	return projects, nil
}

// ComposeClean stops and removes containers + volumes (synchronous).
func (h *DockerHandler) ComposeClean(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, "Project ID is required", model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	path, err := h.resolveProjectPath(r.Context(), r, id)
	if err != nil {
		writeError(w, err.Error(), model.ErrCodeValidationError, http.StatusBadRequest)
		return
	}

	svc := h.getService(r)

	// Log the action
	if lm, err := service.OpenLog(id); err == nil {
		lm.WriteLine("=== Compose Clean (down -v) ===")
		lm.Close()
	}

	result, err := svc.ComposeClean(r.Context(), path)
	if err != nil {
		if lm, err2 := service.OpenLog(id); err2 == nil {
			lm.WriteLine(fmt.Sprintf("Clean failed: %v", err))
			lm.Close()
		}
		writeError(w, result.Message, model.ErrCodeInternalError, http.StatusInternalServerError)
		return
	}

	if lm, err2 := service.OpenLog(id); err2 == nil {
		lm.WriteLine("Compose project cleaned successfully")
		lm.Close()
	}

	writeJSON(w, map[string]string{"status": "cleaned", "message": "Compose project cleaned"}, http.StatusOK)
}
