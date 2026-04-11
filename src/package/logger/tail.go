package logger

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ReadTailLines returns up to the last n lines from path, reading at most maxBytes from the end.
func ReadTailLines(path string, n int, maxBytes int64) ([]string, error) {
	if n <= 0 {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := st.Size()
	start := int64(0)
	if size > maxBytes {
		start = size - maxBytes
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	if start > 0 && len(lines) > 0 {
		lines = lines[1:]
	}
	out := make([]string, 0, n)
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return out, nil
}

// ResolveLogFile returns an absolute path to a log file under the log directory.
func ResolveLogFile(fileName string) (string, error) {
	active := LogFilePath()
	if active == "" {
		return "", errors.New("log file path unavailable")
	}
	dir := filepath.Dir(active)
	base := filepath.Base(strings.TrimSpace(fileName))
	if base == "." || base == "" {
		return "", errors.New("invalid file name")
	}
	if strings.ContainsAny(base, `/\`) {
		return "", errors.New("invalid file name")
	}
	if !strings.HasSuffix(strings.ToLower(base), ".log") {
		return "", errors.New("not a log file")
	}
	full := filepath.Join(dir, base)
	cleanDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	cleanFull, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(cleanDir, cleanFull)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", errors.New("invalid path")
	}
	return cleanFull, nil
}

// LogDirectory returns the absolute directory containing the active log file.
func LogDirectory() string {
	p := LogFilePath()
	if p == "" {
		return ""
	}
	return filepath.Dir(p)
}
