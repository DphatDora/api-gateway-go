package gateway

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"api-gateway/config"
)

// ServiceStatus represents the health state of a downstream service.
type ServiceStatus struct {
	Name         string    `json:"name"`
	Prefix       string    `json:"prefix"`
	Target       string    `json:"target"`
	MonitorPath  string    `json:"monitor_path,omitempty"`
	Status       string    `json:"status"` // "UP" or "DOWN"
	ResponseTime int64     `json:"response_time_ms"`
	LastCheck    string    `json:"last_check"`
	Error        string    `json:"error,omitempty"`
	UpSince      time.Time `json:"-"`
	UptimeStr    string    `json:"uptime,omitempty"`
}

// HealthChecker periodically checks the health of downstream services.
type HealthChecker struct {
	services []config.ServiceTarget
	mu       sync.RWMutex
	statuses map[string]*ServiceStatus
	client   *http.Client
	stopCh   chan struct{}
}

// NewHealthChecker creates a new health checker for the given services.
func NewHealthChecker(services []config.ServiceTarget) *HealthChecker {
	hc := &HealthChecker{
		services: services,
		statuses: make(map[string]*ServiceStatus),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		stopCh: make(chan struct{}),
	}

	// Initialize all services as UNKNOWN
	for _, svc := range services {
		hc.statuses[svc.Name] = &ServiceStatus{
			Name:        svc.Name,
			Prefix:      svc.Prefix,
			Target:      svc.Target,
			MonitorPath: svc.MonitorPath,
			Status:      "UNKNOWN",
		}
	}

	return hc
}

// Start begins periodic health checking (every interval).
func (hc *HealthChecker) Start(interval time.Duration) {
	// Run immediately on start
	hc.checkAll()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				hc.checkAll()
			case <-hc.stopCh:
				return
			}
		}
	}()
}

// Stop terminates the health checker.
func (hc *HealthChecker) Stop() {
	close(hc.stopCh)
}

func (hc *HealthChecker) checkAll() {
	for _, svc := range hc.services {
		go hc.checkService(svc)
	}
}

// CheckServiceNow refreshes and returns the current status for a single service.
func (hc *HealthChecker) CheckServiceNow(name string) (ServiceStatus, bool) {
	var target config.ServiceTarget
	found := false
	for _, svc := range hc.services {
		if svc.Name == name {
			target = svc
			found = true
			break
		}
	}
	if !found {
		return ServiceStatus{}, false
	}

	hc.checkService(target)

	hc.mu.RLock()
	defer hc.mu.RUnlock()
	status, ok := hc.statuses[name]
	if !ok {
		return ServiceStatus{}, false
	}
	return *status, true
}

func (hc *HealthChecker) checkService(svc config.ServiceTarget) {
	// Build health check URL from target base
	// Target is like http://localhost:8046/api/v1, healthPath is /api/v1/health
	// We need the base URL (scheme + host) + healthPath
	baseURL := extractBaseURL(svc.Target)
	healthURL := baseURL + svc.HealthPath

	start := time.Now()
	resp, err := hc.client.Get(healthURL)
	elapsed := time.Since(start).Milliseconds()

	hc.mu.Lock()
	defer hc.mu.Unlock()

	status := hc.statuses[svc.Name]
	status.MonitorPath = svc.MonitorPath
	status.ResponseTime = elapsed
	status.LastCheck = time.Now().UTC().Format(time.RFC3339)

	if err != nil {
		status.Status = "DOWN"
		status.Error = err.Error()
		status.UpSince = time.Time{}
		status.UptimeStr = ""
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if status.Status != "UP" {
			status.UpSince = time.Now()
		}
		status.Status = "UP"
		status.Error = ""
		uptimeDur := time.Since(status.UpSince).Round(time.Second)
		status.UptimeStr = uptimeDur.String()
	} else {
		status.Status = "DOWN"
		status.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		status.UpSince = time.Time{}
		status.UptimeStr = ""
	}
}

// GetStatuses returns a snapshot of all service statuses.
func (hc *HealthChecker) GetStatuses() []ServiceStatus {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	result := make([]ServiceStatus, 0, len(hc.statuses))
	for _, svc := range hc.services {
		if s, ok := hc.statuses[svc.Name]; ok {
			result = append(result, *s)
		}
	}
	return result
}

// extractBaseURL extracts scheme + host from a full URL string.
func extractBaseURL(rawURL string) string {
	// Simple extraction: find the third slash
	// e.g., http://localhost:8046/api/v1 -> http://localhost:8046
	count := 0
	for i, ch := range rawURL {
		if ch == '/' {
			count++
			if count == 3 {
				return rawURL[:i]
			}
		}
	}
	return rawURL
}
