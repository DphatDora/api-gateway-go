package gateway

import (
	"encoding/json"
	"runtime"
	"sync"
	"time"

	"api-gateway/package/logger"
)

// RequestRecord represents a single request that passed through the gateway.
type RequestRecord struct {
	Timestamp   string            `json:"timestamp"`
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	QueryString string            `json:"query_string,omitempty"`
	Service     string            `json:"service"`
	StatusCode  int               `json:"status_code"`
	LatencyMS   int64             `json:"latency_ms"`
	ClientIP    string            `json:"client_ip"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// ServiceMetrics holds per-service request metrics.
type ServiceMetrics struct {
	Name           string  `json:"name"`
	RequestCount   int64   `json:"request_count"`
	AvgLatencyMS   float64 `json:"avg_latency_ms"`
	MinLatencyMS   float64 `json:"min_latency_ms"`
	MaxLatencyMS   float64 `json:"max_latency_ms"`
	Status2xx      int64   `json:"status_2xx"`
	Status3xx      int64   `json:"status_3xx"`
	Status4xx      int64   `json:"status_4xx"`
	Status5xx      int64   `json:"status_5xx"`
	RequestsPerMin float64 `json:"requests_per_min"`
}

// MetricsSnapshot is the full metrics response for the dashboard API.
type MetricsSnapshot struct {
	System   SystemMetrics    `json:"system"`
	Services []ServiceMetrics `json:"services"`
	Total    TotalMetrics     `json:"total"`
}

// SystemMetrics holds Go runtime metrics.
type SystemMetrics struct {
	NumGoroutine int            `json:"num_goroutine"`
	Memory       MemoryMetrics  `json:"memory"`
	Uptime       string         `json:"uptime"`
}

// MemoryMetrics holds memory usage stats.
type MemoryMetrics struct {
	AllocBytes     uint64 `json:"alloc_bytes"`
	HeapInuseBytes uint64 `json:"heap_inuse_bytes"`
	HeapSysBytes   uint64 `json:"heap_sys_bytes"`
	StackInuseBytes uint64 `json:"stack_inuse_bytes"`
	NumGC          uint32 `json:"num_gc"`
}

// TotalMetrics holds aggregate metrics across all services.
type TotalMetrics struct {
	RequestCount   int64   `json:"request_count"`
	RequestsPerMin float64 `json:"requests_per_min"`
	AvgLatencyMS   float64 `json:"avg_latency_ms"`
}

// requestBucket holds per-second request data.
type requestBucket struct {
	sec       int64
	count     int64
	latencyNS int64
	minLat    int64
	maxLat    int64
	s2xx      int64
	s3xx      int64
	s4xx      int64
	s5xx      int64
}

const (
	bucketWindow    = 60
	ringBufferSize  = 500
)

// MetricsCollector collects request metrics per service and maintains a request log ring buffer.
type MetricsCollector struct {
	mu             sync.Mutex
	serviceBuckets map[string]*[bucketWindow]requestBucket

	ringMu   sync.RWMutex
	ring     [ringBufferSize]RequestRecord
	ringHead int
	ringLen  int

	startTime time.Time
}

// NewMetricsCollector creates a new MetricsCollector.
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		serviceBuckets: make(map[string]*[bucketWindow]requestBucket),
		startTime:      time.Now(),
	}
}

// RecordRequest records a request's metrics.
func (mc *MetricsCollector) RecordRequest(serviceName, method, path, clientIP string, statusCode int, latency time.Duration, headers map[string]string, queryString string) {
	now := time.Now()
	latencyNS := latency.Nanoseconds()
	if latencyNS < 0 {
		latencyNS = 0
	}

	// Update per-service buckets
	mc.mu.Lock()
	buckets, ok := mc.serviceBuckets[serviceName]
	if !ok {
		buckets = &[bucketWindow]requestBucket{}
		mc.serviceBuckets[serviceName] = buckets
	}

	idx := int(now.Unix() % int64(bucketWindow))
	b := &buckets[idx]
	if b.sec != now.Unix() {
		*b = requestBucket{sec: now.Unix()}
	}
	b.count++
	b.latencyNS += latencyNS
	if b.minLat == 0 || latencyNS < b.minLat {
		b.minLat = latencyNS
	}
	if latencyNS > b.maxLat {
		b.maxLat = latencyNS
	}

	switch {
	case statusCode >= 200 && statusCode < 300:
		b.s2xx++
	case statusCode >= 300 && statusCode < 400:
		b.s3xx++
	case statusCode >= 400 && statusCode < 500:
		b.s4xx++
	case statusCode >= 500:
		b.s5xx++
	}
	mc.mu.Unlock()

	// Add to ring buffer
	record := RequestRecord{
		Timestamp:   now.UTC().Format(time.RFC3339),
		Method:      method,
		Path:        path,
		QueryString: queryString,
		Service:     serviceName,
		StatusCode:  statusCode,
		LatencyMS:   latency.Milliseconds(),
		ClientIP:    clientIP,
		Headers:     headers,
	}

	mc.ringMu.Lock()
	mc.ring[mc.ringHead] = record
	mc.ringHead = (mc.ringHead + 1) % ringBufferSize
	if mc.ringLen < ringBufferSize {
		mc.ringLen++
	}
	mc.ringMu.Unlock()

	// Write to persistent log file
	if jsonData, err := json.Marshal(record); err == nil {
		// Add newline for JSON lines format
		jsonData = append(jsonData, '\n')
		logger.WriteRequestLog(jsonData)
	}
}

// GetRecentRequests returns the most recent N requests (newest first).
func (mc *MetricsCollector) GetRecentRequests(limit int) []RequestRecord {
	mc.ringMu.RLock()
	defer mc.ringMu.RUnlock()

	if limit <= 0 || limit > mc.ringLen {
		limit = mc.ringLen
	}

	result := make([]RequestRecord, limit)
	for i := 0; i < limit; i++ {
		idx := (mc.ringHead - 1 - i + ringBufferSize) % ringBufferSize
		result[i] = mc.ring[idx]
	}
	return result
}

// Snapshot returns the current metrics for all services.
func (mc *MetricsCollector) Snapshot() MetricsSnapshot {
	now := time.Now().Unix()

	mc.mu.Lock()
	defer mc.mu.Unlock()

	var services []ServiceMetrics
	var totalCount int64
	var totalLatency int64

	for name, buckets := range mc.serviceBuckets {
		sm := ServiceMetrics{Name: name}
		var minLat, maxLat int64

		for i := range buckets {
			b := &buckets[i]
			if now-b.sec >= int64(bucketWindow) {
				continue
			}
			sm.RequestCount += b.count
			totalLatency += b.latencyNS
			if b.minLat > 0 && (minLat == 0 || b.minLat < minLat) {
				minLat = b.minLat
			}
			if b.maxLat > maxLat {
				maxLat = b.maxLat
			}
			sm.Status2xx += b.s2xx
			sm.Status3xx += b.s3xx
			sm.Status4xx += b.s4xx
			sm.Status5xx += b.s5xx
		}

		if sm.RequestCount > 0 {
			svcLatency := int64(0)
			for i := range buckets {
				if now-buckets[i].sec < int64(bucketWindow) {
					svcLatency += buckets[i].latencyNS
				}
			}
			sm.AvgLatencyMS = float64(svcLatency) / float64(sm.RequestCount) / float64(time.Millisecond)
			sm.MinLatencyMS = float64(minLat) / float64(time.Millisecond)
			sm.MaxLatencyMS = float64(maxLat) / float64(time.Millisecond)
		}
		sm.RequestsPerMin = float64(sm.RequestCount)
		totalCount += sm.RequestCount
		services = append(services, sm)
	}

	// System metrics
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	uptime := time.Since(mc.startTime).Round(time.Second).String()

	totalAvgLat := float64(0)
	if totalCount > 0 {
		totalAvgLat = float64(totalLatency) / float64(totalCount) / float64(time.Millisecond)
	}

	return MetricsSnapshot{
		System: SystemMetrics{
			NumGoroutine: runtime.NumGoroutine(),
			Memory: MemoryMetrics{
				AllocBytes:      memStats.Alloc,
				HeapInuseBytes:  memStats.HeapInuse,
				HeapSysBytes:    memStats.HeapSys,
				StackInuseBytes: memStats.StackInuse,
				NumGC:           memStats.NumGC,
			},
			Uptime: uptime,
		},
		Services: services,
		Total: TotalMetrics{
			RequestCount:   totalCount,
			RequestsPerMin: float64(totalCount),
			AvgLatencyMS:   totalAvgLat,
		},
	}
}
