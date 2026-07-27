package service

import (
	"context"
	"os"
	"path/filepath"

	"dockerbox/backend/internal/model"
)

// HostFileAccess abstracts file operations on a Docker host.
type HostFileAccess interface {
	List(ctx context.Context, hostPath string) ([]model.FileInfo, error)
	GetInfo(ctx context.Context, hostPath string) (*model.FileInfo, error)
	Read(ctx context.Context, hostPath string) ([]byte, error)
	ReadRange(ctx context.Context, hostPath string, offset, length int64) ([]byte, error)
	GetSize(ctx context.Context, hostPath string) (int64, error)
	Write(ctx context.Context, hostPath string, data []byte) error
	Remove(ctx context.Context, hostPath string) error
	Rename(ctx context.Context, oldPath, newPath string) error
	Mkdir(ctx context.Context, hostPath string) error
}

// ---------------------------------------------------------------------------
// SSH (SFTP) — remote host via SSH
// ---------------------------------------------------------------------------

type SSHFileAccess struct{ docker *DockerService }

func NewSSHFileAccess(d *DockerService) *SSHFileAccess { return &SSHFileAccess{docker: d} }

func (a *SSHFileAccess) List(ctx context.Context, p string) ([]model.FileInfo, error) {
	return a.docker.ListFilesViaDocker(ctx, p)
}
func (a *SSHFileAccess) GetInfo(ctx context.Context, p string) (*model.FileInfo, error) {
	return a.docker.GetFileInfoViaDocker(ctx, p)
}
func (a *SSHFileAccess) Read(ctx context.Context, p string) ([]byte, error) {
	return a.docker.ReadFileViaDocker(ctx, p)
}
func (a *SSHFileAccess) ReadRange(ctx context.Context, p string, o, l int64) ([]byte, error) {
	return a.docker.ReadFileRangeViaSFTP(ctx, p, o, l)
}
func (a *SSHFileAccess) GetSize(ctx context.Context, p string) (int64, error) {
	return a.docker.GetFileSizeViaSFTP(ctx, p)
}
func (a *SSHFileAccess) Write(ctx context.Context, p string, d []byte) error {
	return a.docker.WriteFileViaSFTP(ctx, p, d)
}
func (a *SSHFileAccess) Remove(ctx context.Context, p string) error {
	return a.docker.RemoveViaSFTP(ctx, p)
}
func (a *SSHFileAccess) Rename(ctx context.Context, o, n string) error {
	return a.docker.RenameViaSFTP(ctx, o, n)
}
func (a *SSHFileAccess) Mkdir(ctx context.Context, p string) error {
	return a.docker.MkdirViaSFTP(ctx, p)
}

// ---------------------------------------------------------------------------
// Socket / TCP — local filesystem access
//
// Both scenarios:
//  1. Binary runs on host directly → files are local
//  2. Binary runs in container with sock + volumes mapped → files are local via mounts
//
// No Docker exec needed. Just use os package.
// ---------------------------------------------------------------------------

type SocketFileAccess struct{}

func NewSocketFileAccess() *SocketFileAccess {
	return &SocketFileAccess{}
}

func (a *SocketFileAccess) List(_ context.Context, hostPath string) ([]model.FileInfo, error) {
	entries, err := os.ReadDir(hostPath)
	if err != nil {
		return nil, err
	}
	files := make([]model.FileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, model.FileInfo{
			Name:        info.Name(),
			Path:        filepath.Join(hostPath, info.Name()),
			Size:        info.Size(),
			IsDir:       info.IsDir(),
			ModTime:     info.ModTime(),
			Permissions: info.Mode().String(),
		})
	}
	return files, nil
}

func (a *SocketFileAccess) GetInfo(_ context.Context, hostPath string) (*model.FileInfo, error) {
	info, err := os.Stat(hostPath)
	if err != nil {
		return nil, err
	}
	return &model.FileInfo{
		Name:        info.Name(),
		Path:        hostPath,
		Size:        info.Size(),
		IsDir:       info.IsDir(),
		ModTime:     info.ModTime(),
		Permissions: info.Mode().String(),
	}, nil
}

func (a *SocketFileAccess) Read(_ context.Context, hostPath string) ([]byte, error) {
	return os.ReadFile(hostPath)
}

func (a *SocketFileAccess) ReadRange(_ context.Context, hostPath string, offset, length int64) ([]byte, error) {
	f, err := os.Open(hostPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := make([]byte, length)
	n, err := f.ReadAt(buf, offset)
	if err != nil && err.Error() != "EOF" {
		return nil, err
	}
	return buf[:n], nil
}

func (a *SocketFileAccess) GetSize(_ context.Context, hostPath string) (int64, error) {
	info, err := os.Stat(hostPath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (a *SocketFileAccess) Write(_ context.Context, hostPath string, data []byte) error {
	// Ensure parent directory exists
	dir := filepath.Dir(hostPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(hostPath, data, 0644)
}

func (a *SocketFileAccess) Remove(_ context.Context, hostPath string) error {
	return os.RemoveAll(hostPath)
}

func (a *SocketFileAccess) Rename(_ context.Context, oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (a *SocketFileAccess) Mkdir(_ context.Context, hostPath string) error {
	return os.MkdirAll(hostPath, 0755)
}

var _ HostFileAccess = (*SSHFileAccess)(nil)
var _ HostFileAccess = (*SocketFileAccess)(nil)
