package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"api-gateway/config"
	"api-gateway/internal/gateway"
	"api-gateway/package/logger"
	"api-gateway/web/dashboard"

	"github.com/gin-gonic/gin"
)

// DashboardHandler serves the gateway dashboard and its API endpoints.
type DashboardHandler struct {
	healthChecker    *gateway.HealthChecker
	metricsCollector *gateway.MetricsCollector
}

// NewDashboardHandler creates a new dashboard handler.
func NewDashboardHandler(hc *gateway.HealthChecker, mc *gateway.MetricsCollector) *DashboardHandler {
	return &DashboardHandler{
		healthChecker:    hc,
		metricsCollector: mc,
	}
}

// ServeDashboard serves the embedded HTML dashboard.
func (h *DashboardHandler) ServeDashboard(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", dashboard.DashboardHTML)
}

// GetServices returns the list of services with their health status.
func (h *DashboardHandler) GetServices(c *gin.Context) {
	statuses := h.healthChecker.GetStatuses()
	c.JSON(http.StatusOK, gin.H{
		"services": statuses,
	})
}

// GetMetrics returns the current metrics snapshot.
func (h *DashboardHandler) GetMetrics(c *gin.Context) {
	snap := h.metricsCollector.Snapshot()
	c.JSON(http.StatusOK, snap)
}

// GetRequests returns recent requests from the ring buffer.
func (h *DashboardHandler) GetRequests(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "100")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	records := h.metricsCollector.GetRecentRequests(limit)
	c.JSON(http.StatusOK, gin.H{
		"requests": records,
		"count":    len(records),
	})
}

// ── Log endpoints ──

// GetLogFiles lists log files in the log directory.
func (h *DashboardHandler) GetLogFiles(c *gin.Context) {
	files, err := logger.ListLogFiles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"dir":   logger.LogDirectory(),
		"files": files,
	})
}

// GetLogs returns the last N lines from a chosen log file as JSON.
// Query params:
//   - file  : base name of the log file (default: active log file)
//   - lines : max lines to return (default 200, max 2000)
//   - search: optional search string (case-insensitive substring match)
//   - level : one of "DEBUG", "INFO", "WARN", "ERROR" (default: all)
func (h *DashboardHandler) GetLogs(c *gin.Context) {
	active := logger.LogFilePath()
	if active == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "log file path unavailable"})
		return
	}

	fileParam := c.Query("file")
	if fileParam == "" {
		fileParam = filepath.Base(active)
	}

	path, err := logger.ResolveLogFile(fileParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	n, _ := strconv.Atoi(c.DefaultQuery("lines", "200"))
	if n < 1 {
		n = 200
	}
	if n > 2000 {
		n = 2000
	}

	search := strings.TrimSpace(c.Query("search"))
	level := strings.ToUpper(strings.TrimSpace(c.Query("level")))

	const maxRead = 2 * 1024 * 1024

	// When search or level filter is applied, read more lines to increase chance of matches
	readN := n
	if search != "" || level != "" {
		readN = 10000
	}

	lines, err := logger.ReadTailLines(path, readN, maxRead)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Reverse so newest first
	reverseLines(lines)

	if search != "" || level != "" {
		lines = filterLogLines(lines, search, level)
		if len(lines) > n {
			lines = lines[:n]
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"path":   path,
		"lines":  lines,
		"search": search,
		"level":  level,
	})
}

// DashboardAuth protects dashboard routes when dashboardToken is set.
// Token may be sent as query ?token= or header X-Dashboard-Token.
func DashboardAuth(conf *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		expected := strings.TrimSpace(conf.Log.DashboardToken)
		if expected == "" {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		t := strings.TrimSpace(c.Query("token"))
		if t == "" {
			t = strings.TrimSpace(c.GetHeader("X-Dashboard-Token"))
		}
		if t != expected {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}

// ── Log filtering helpers ──

func reverseLines(lines []string) {
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
}

func filterLogLines(lines []string, search string, level string) []string {
	searchLower := strings.ToLower(search)
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if !matchLogLevel(l, level) {
			continue
		}
		if search == "" || strings.Contains(strings.ToLower(l), searchLower) {
			out = append(out, l)
		}
	}
	return out
}

func matchLogLevel(raw string, level string) bool {
	if level == "" {
		return true
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return strings.Contains(strings.ToUpper(raw), `"LEVEL":"`+level+`"`)
	}
	v, ok := obj["level"]
	if !ok {
		return false
	}
	s, ok := v.(string)
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(s), level)
}
