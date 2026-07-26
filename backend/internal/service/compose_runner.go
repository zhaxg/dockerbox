package service

import (
		"context"
	"fmt"
	"os"
	"strconv"
	"io"
	"regexp"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ComposeRunStatus represents the status of a compose operation.
type ComposeRunStatus string

const (
	ComposeRunning  ComposeRunStatus = "running"
	ComposeFinished ComposeRunStatus = "finished"
	ComposeFailed   ComposeRunStatus = "failed"
	ComposeAborted  ComposeRunStatus = "aborted"
)

// ComposeRun represents an active compose operation.
type ComposeRun struct {
	ID        string
	Status    ComposeRunStatus
	Output    []string
	Done      chan struct{}
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	mu        sync.Mutex
	startTime time.Time
	sshHost   string
	sshKey    string
	runtime   string
	logMgr    *ComposeLogManager
}

// ComposeRunner manages active compose operations.
type ComposeRunner struct {
	runs map[string]*ComposeRun
	mu   sync.RWMutex
}

var composeRunnerInstance *ComposeRunner
var composeRunnerOnce sync.Once

// GetComposeRunner returns the singleton ComposeRunner.
func GetComposeRunner() *ComposeRunner {
	composeRunnerOnce.Do(func() {
		composeRunnerInstance = &ComposeRunner{
			runs: make(map[string]*ComposeRun),
		}
	})
	return composeRunnerInstance
}

// Start launches a compose command and streams output.
func (r *ComposeRunner) Start(id string, sshHost string, sshKey string, runtime string, args []string, workDir string) *ComposeRun {
	r.mu.Lock()
	// Abort existing run for same ID
	if existing, ok := r.runs[id]; ok {
		existing.Abort()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	var cmd *exec.Cmd
	if sshHost != "" {
		// For SSH, embed workDir in the command
		host := sshHost
		port := "22"
		if idx := strings.LastIndex(host, ":"); idx > 0 {
			if _, err := strconv.Atoi(host[idx+1:]); err == nil {
				port = host[idx+1:]
				host = host[:idx]
			}
		}
		keyFile := WriteSSHKeyTemp(sshKey)
		sshArgs := []string{"-i", keyFile, "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-p", port, host, "cd "+workDir+" && PODMAN_COMPOSE_WARNING_LOGS=false "+runtime+" compose "+strings.Join(args, " ")}
		cmd = exec.CommandContext(ctx, "ssh", sshArgs...)
	} else {
		cmd = composeCommand(ctx, runtime, args...)
		cmd.Dir = workDir
	}

	run := &ComposeRun{
		ID:        id,
		Status:    ComposeRunning,
		Output:    []string{},
		Done:      make(chan struct{}),
		cmd:       cmd,
		cancel:    cancel,
		startTime: time.Now(),
		sshHost:   sshHost,
		runtime:   runtime,
		sshKey:    sshKey,
	}
	// Open log file for this project
	logMgr, err := OpenLog(id)
	if err == nil {
		run.logMgr = logMgr
	}

	r.runs[id] = run
	r.mu.Unlock()

	go r.execute(run)

	return run
}

var ansiRegex = regexp.MustCompile(`\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// execute runs the command and streams output line by line.
func (r *ComposeRunner) execute(run *ComposeRun) {
	defer close(run.Done)
	defer func() {
		if run.logMgr != nil {
			run.logMgr.Close()
		}
		run.mu.Lock()
		if run.Status == ComposeRunning {
			if run.cmd.ProcessState != nil && run.cmd.ProcessState.ExitCode() != 0 {
				run.Status = ComposeFailed
			} else {
				run.Status = ComposeFinished
			}
		}
		run.mu.Unlock()
		// Clean up after a delay (allow reconnection)
		time.AfterFunc(5*time.Minute, func() {
			r.mu.Lock()
			delete(r.runs, run.ID)
			r.mu.Unlock()
		})
	}()

	// Use pipes to read output line by line
	stdoutPipe, err := run.cmd.StdoutPipe()
	if err != nil {
		run.mu.Lock()
		run.Output = append(run.Output, fmt.Sprintf("Error creating stdout pipe: %v", err))
		run.Status = ComposeFailed
		run.mu.Unlock()
		return
	}
	stderrPipe, err := run.cmd.StderrPipe()
	if err != nil {
		run.mu.Lock()
		run.Output = append(run.Output, fmt.Sprintf("Error creating pipe: %v", err))
		run.Status = ComposeFailed
		run.mu.Unlock()
		return
	}

	if err := run.cmd.Start(); err != nil {
		run.mu.Lock()
		run.Output = append(run.Output, fmt.Sprintf("Failed to start: %v", err))
		run.Status = ComposeFailed
		run.mu.Unlock()
		return
	}

	// Read stdout and stderr concurrently
	var wg sync.WaitGroup
	scan := func(reader io.Reader) {
		defer wg.Done()
		buf := make([]byte, 4096)
		var leftover string
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				leftover += string(buf[:n])
				// Split by newline or carriage return
				for {
					idxN := strings.IndexByte(leftover, '\n')
					idxR := strings.IndexByte(leftover, '\r')
					if idxN == -1 && idxR == -1 {
						break
					}
					idx := len(leftover)
					isN := false
					if idxN != -1 && (idxR == -1 || idxN <= idxR) {
						idx = idxN
						isN = true
					} else if idxR != -1 {
						idx = idxR
						isN = false
					}
					line := stripANSI(leftover[:idx])
					leftover = leftover[idx+1:]
					run.mu.Lock()
					if isN {
						// Newline: append as new line
						if line != "" {
							run.Output = append(run.Output, line)
							if run.logMgr != nil {
								run.logMgr.WriteLine(line)
							}
						}
					} else {
						// Carriage return: replace last line (progress update)
						if line != "" {
							if len(run.Output) > 0 {
								run.Output[len(run.Output)-1] = line
							} else {
								run.Output = append(run.Output, line)
							}
							if run.logMgr != nil {
								run.logMgr.WriteLine(line)
							}
						}
					}
					run.mu.Unlock()
				}
			}
			if err != nil {
				if leftover != "" {
					line := stripANSI(leftover)
					if line != "" {
						run.mu.Lock()
						run.Output = append(run.Output, line)
						run.mu.Unlock()
					}
				}
				break
			}
		}
	}

	wg.Add(2)
	go scan(stdoutPipe)
	go scan(stderrPipe)
	wg.Wait()

	waitErr := run.cmd.Wait()
	run.mu.Lock()
	run.Status = ComposeFinished
	if waitErr != nil {
		run.Status = ComposeFailed
		// Add exit info if not already captured
		exitMsg := fmt.Sprintf("Exit: %v", waitErr)
		found := false
		for _, line := range run.Output {
			if strings.Contains(line, "Exit") {
				found = true
				break
			}
		}
		if !found {
			run.Output = append(run.Output, exitMsg)
		}
	}
	run.mu.Unlock()
}

// Get returns an active run by ID.
func (r *ComposeRunner) Get(id string) *ComposeRun {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.runs[id]
}

// GetOutput returns a copy of the output lines.
func (run *ComposeRun) GetOutput() []string {
	run.mu.Lock()
	defer run.mu.Unlock()
	out := make([]string, len(run.Output))
	copy(out, run.Output)
	return out
}

// GetStatus returns the current status.
func (run *ComposeRun) GetStatus() ComposeRunStatus {
	run.mu.Lock()
	defer run.mu.Unlock()
	return run.Status
}

// Abort kills the running process.
func (run *ComposeRun) Abort() {
	run.mu.Lock()
	defer run.mu.Unlock()
	if run.Status == ComposeRunning {
		run.Status = ComposeAborted
		if run.cancel != nil {
			run.cancel()
		}
		if run.cmd != nil && run.cmd.Process != nil {
			run.cmd.Process.Kill()
		}
	}
}

// IsRunning returns whether the process is still running.
func (run *ComposeRun) IsRunning() bool {
	return run.GetStatus() == ComposeRunning
}

// Elapsed returns the time since the operation started.
func (run *ComposeRun) Elapsed() time.Duration {
	return time.Since(run.startTime)
}

// StartRedeploy runs down, pull, up sequentially.
func (r *ComposeRunner) StartRedeploy(id string, sshHost string, sshKey string, runtime string, workDir string) *ComposeRun {
	r.mu.Lock()
	if existing, ok := r.runs[id]; ok {
		existing.Abort()
	}

	_, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	run := &ComposeRun{
		ID:        id,
		Status:    ComposeRunning,
		Output:    []string{},
		Done:      make(chan struct{}),
		cancel:    cancel,
		startTime: time.Now(),
		sshHost:   sshHost,
		runtime:   runtime,
	}
	r.runs[id] = run
	r.mu.Unlock()

	go r.executeRedeploy(run, workDir, sshHost, sshKey)

	return run
}

// executeRedeploy runs down -> pull -> up sequentially.
func (r *ComposeRunner) executeRedeploy(run *ComposeRun, workDir string, sshHost string, sshKey string) {
	defer close(run.Done)
	defer func() {
		if run.logMgr != nil {
			run.logMgr.Close()
		}
		run.mu.Lock()
		if run.Status == ComposeRunning {
			run.Status = ComposeFinished
		}
		run.mu.Unlock()
		time.AfterFunc(5*time.Minute, func() {
			r.mu.Lock()
			delete(r.runs, run.ID)
			r.mu.Unlock()
		})
	}()

	steps := []struct {
		name string
		args []string
	}{
		{"[1/3] 停止并清理容器", []string{"down"}},
		{"[2/3] 拉取最新镜像", []string{"pull"}},
		{"[3/3] 启动服务", []string{"up", "-d"}},
	}

	for _, step := range steps {
		if run.GetStatus() != ComposeRunning {
			return
		}
		run.mu.Lock()
		run.Output = append(run.Output, step.name)
		run.mu.Unlock()

		var cmd *exec.Cmd
		if sshHost != "" {
			host := sshHost
			port := "22"
			if idx := strings.LastIndex(host, ":"); idx > 0 {
				if _, err := strconv.Atoi(host[idx+1:]); err == nil {
					port = host[idx+1:]
					host = host[:idx]
				}
			}
			keyFile := WriteSSHKeyTemp(sshKey)
			defer os.Remove(keyFile)
			sshArgs := []string{"-i", keyFile, "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-p", port, host, "cd " + workDir + " && PODMAN_COMPOSE_WARNING_LOGS=false " + run.runtime + " compose " + strings.Join(step.args, " ")}
			cmd = exec.CommandContext(context.Background(), "ssh", sshArgs...)
		} else {
			cmd = composeCommand(context.Background(), run.runtime, step.args...)
			cmd.Dir = workDir
		}

		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			run.mu.Lock()
			run.Output = append(run.Output, fmt.Sprintf("Error: %v", err))
			run.mu.Unlock()
			run.Status = ComposeFailed
			return
		}
		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			run.mu.Lock()
			run.Output = append(run.Output, fmt.Sprintf("Error: %v", err))
			run.mu.Unlock()
			run.Status = ComposeFailed
			return
		}

		if err := cmd.Start(); err != nil {
			run.mu.Lock()
			run.Output = append(run.Output, fmt.Sprintf("Failed to start: %v", err))
			run.mu.Unlock()
			run.Status = ComposeFailed
			return
		}

		var wg sync.WaitGroup
		scan := func(reader io.Reader) {
			defer wg.Done()
			buf := make([]byte, 4096)
			var leftover string
			for {
				n, err := reader.Read(buf)
				if n > 0 {
					leftover += string(buf[:n])
					for {
						idxN := strings.IndexByte(leftover, '\n')
						idxR := strings.IndexByte(leftover, '\r')
						if idxN == -1 && idxR == -1 {
							break
						}
						idx := len(leftover)
						isN := false
						if idxN != -1 && (idxR == -1 || idxN <= idxR) {
							idx = idxN
							isN = true
						} else if idxR != -1 {
							idx = idxR
						}
						line := stripANSI(leftover[:idx])
						leftover = leftover[idx+1:]
						run.mu.Lock()
						if isN {
							if line != "" {
								run.Output = append(run.Output, line)
							}
						} else {
							if line != "" {
								if len(run.Output) > 0 {
									run.Output[len(run.Output)-1] = line
								} else {
									run.Output = append(run.Output, line)
								}
							}
						}
						run.mu.Unlock()
					}
				}
				if err != nil {
					if leftover != "" {
						line := stripANSI(leftover)
						if line != "" {
							run.mu.Lock()
							run.Output = append(run.Output, line)
							run.mu.Unlock()
						}
					}
					break
				}
			}
		}

		wg.Add(2)
		go scan(stdoutPipe)
		go scan(stderrPipe)
		wg.Wait()

		waitErr := cmd.Wait()
		if waitErr != nil {
			run.mu.Lock()
			run.Output = append(run.Output, fmt.Sprintf("Step failed: %v", waitErr))
			run.mu.Unlock()
			run.Status = ComposeFailed
			return
		}
	}
}

// StartLogs tails compose logs and streams output.
func (r *ComposeRunner) StartLogs(id string, sshHost string, sshKey string, runtime string, workDir string) *ComposeRun {
	r.mu.Lock()
	if existing, ok := r.runs[id]; ok {
		if existing.GetStatus() == ComposeRunning {
			r.mu.Unlock()
			return existing
		}
	}

	_, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	run := &ComposeRun{
		ID:        id,
		Status:    ComposeRunning,
		Output:    []string{},
		Done:      make(chan struct{}),
		cancel:    cancel,
		startTime: time.Now(),
		sshHost:   sshHost,
		runtime:   runtime,
	}
	r.runs[id] = run
	r.mu.Unlock()

	go r.executeLogs(run, workDir, sshHost, sshKey)

	return run
}

// executeLogs runs docker compose logs -f --tail 200 and streams output.
func (r *ComposeRunner) executeLogs(run *ComposeRun, workDir string, sshHost string, sshKey string) {
	defer close(run.Done)
	defer func() {
		run.mu.Lock()
		run.Status = ComposeFinished
		run.mu.Unlock()
		if run.logMgr != nil {
			run.logMgr.Close()
		}
		time.AfterFunc(5*time.Minute, func() {
			r.mu.Lock()
			delete(r.runs, run.ID)
			r.mu.Unlock()
		})
	}()

	var cmd *exec.Cmd
	if sshHost != "" {
		host := sshHost
		port := "22"
		if idx := strings.LastIndex(host, ":"); idx > 0 {
			if _, err := strconv.Atoi(host[idx+1:]); err == nil {
				port = host[idx+1:]
				host = host[:idx]
			}
		}
		keyFile := WriteSSHKeyTemp(sshKey)
		sshArgs := []string{"-i", keyFile, "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-p", port, host, "cd " + workDir + " && PODMAN_COMPOSE_WARNING_LOGS=false " + run.runtime + " compose logs -f --tail 200"}
		cmd = exec.CommandContext(context.Background(), "ssh", sshArgs...)
	} else {
		cmd = composeCommand(context.Background(), run.runtime, "logs", "-f", "--tail", "200")
		cmd.Dir = workDir
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		run.mu.Lock()
		run.Output = append(run.Output, fmt.Sprintf("Error: %v", err))
		run.mu.Unlock()
		return
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		run.mu.Lock()
		run.Output = append(run.Output, fmt.Sprintf("Error: %v", err))
		run.mu.Unlock()
		return
	}

	if err := cmd.Start(); err != nil {
		run.mu.Lock()
		run.Output = append(run.Output, fmt.Sprintf("Failed to start: %v", err))
		run.mu.Unlock()
		return
	}

	var wg sync.WaitGroup
	scan := func(reader io.Reader) {
		defer wg.Done()
		buf := make([]byte, 4096)
		var leftover string
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				leftover += string(buf[:n])
				for {
					idxN := strings.IndexByte(leftover, '\n')
					idxR := strings.IndexByte(leftover, '\r')
					if idxN == -1 && idxR == -1 {
						break
					}
					idx := len(leftover)
					isN := false
					if idxN != -1 && (idxR == -1 || idxN <= idxR) {
						idx = idxN
						isN = true
					} else if idxR != -1 {
						idx = idxR
					}
					line := stripANSI(leftover[:idx])
					leftover = leftover[idx+1:]
					run.mu.Lock()
					if isN {
						if line != "" {
							run.Output = append(run.Output, line)
						}
					} else {
						if line != "" {
							if len(run.Output) > 0 {
								run.Output[len(run.Output)-1] = line
							} else {
								run.Output = append(run.Output, line)
							}
						}
					}
					run.mu.Unlock()
				}
			}
			if err != nil {
				break
			}
		}
	}

	wg.Add(2)
	go scan(stdoutPipe)
	go scan(stderrPipe)
	wg.Wait()
	cmd.Wait()
}

// writeSSHKey writes the SSH key to a temp file and returns the path.
func WriteSSHKeyTemp(key string) string {
	if key == "" {
		return ""
	}
	tmpFile, err := os.CreateTemp("", "boxbox-ssh-*")
	if err != nil {
		return ""
	}
	tmpFile.WriteString(key)
	tmpFile.Close()
	os.Chmod(tmpFile.Name(), 0600)
	return tmpFile.Name()
}
