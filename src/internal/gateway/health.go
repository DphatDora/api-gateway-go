package gateway

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"api-gateway/config"
	"api-gateway/package/logger"
)

// ServiceStatus represents the health state of a downstream service.
type ServiceStatus struct {
	Name         string `json:"name"`
	Prefix       string `json:"prefix"`
	Target       string `json:"target"`
	Status       string `json:"status"` // "UP" or "DOWN"
	ResponseTime int64  `json:"response_time_ms"`
	LastCheck    string `json:"last_check"`
	Error        string `json:"error,omitempty"`
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
			Name:   svc.Name,
			Prefix: svc.Prefix,
			Target: svc.Target,
			Status: "UNKNOWN",
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
	status.ResponseTime = elapsed
	status.LastCheck = time.Now().UTC().Format(time.RFC3339)

	if err != nil {
		status.Status = "DOWN"
		status.Error = err.Error()
		logger.Warnf("[Health] %s is DOWN: %v", svc.Name, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		status.Status = "UP"
		status.Error = ""
		logger.Debugf("[Health] %s is UP (%dms)", svc.Name, elapsed)
	} else {
		status.Status = "DOWN"
		status.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		logger.Warnf("[Health] %s returned %d", svc.Name, resp.StatusCode)
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
