package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// RotateWriter is a size-based rotating file writer.
type RotateWriter struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	file     *os.File
	size     int64
}

// NewRotateWriter creates or opens the log file and prepares rotation.
func NewRotateWriter(path string, maxBytes int64) (*RotateWriter, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	return &RotateWriter{
		path:     path,
		maxBytes: maxBytes,
		file:     f,
		size:     info.Size(),
	}, nil
}

// Write implements io.Writer. Thread-safe; rotates when maxBytes is exceeded.
func (rw *RotateWriter) Write(p []byte) (int, error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	if rw.size+int64(len(p)) > rw.maxBytes {
		if err := rw.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := rw.file.Write(p)
	rw.size += int64(n)
	return n, err
}

func (rw *RotateWriter) rotate() error {
	_ = rw.file.Close()

	rotated := rw.path + "." + time.Now().UTC().Format("20060102-150405")
	_ = os.Rename(rw.path, rotated)

	f, err := os.OpenFile(rw.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	rw.file = f
	rw.size = 0
	return nil
}

// Path returns the absolute path of the active log file.
func (rw *RotateWriter) Path() string { return rw.path }

// Sync flushes the file to disk.
func (rw *RotateWriter) Sync() error {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	return rw.file.Sync()
}

// LogFileInfo holds metadata about a log file.
type LogFileInfo struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Current bool   `json:"current"`
	ModTime int64  `json:"modTimeUnix,omitempty"`
}

// ListLogFiles returns all log files in the log directory sorted by modification time (newest first).
func ListLogFiles() ([]LogFileInfo, error) {
	activePath := LogFilePath()
	if activePath == "" {
		return nil, nil
	}
	dir := filepath.Dir(activePath)
	baseName := filepath.Base(activePath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []LogFileInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".log") {
			continue
		}
		info, err2 := e.Info()
		if err2 != nil {
			continue
		}
		files = append(files, LogFileInfo{
			Name:    name,
			Size:    info.Size(),
			Current: name == baseName,
			ModTime: info.ModTime().Unix(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].Current {
			return true
		}
		if files[j].Current {
			return false
		}
		return files[i].Name > files[j].Name
	})

	return files, nil
}
