package middleware

import (
	"time"

	"api-gateway/internal/gateway"

	"github.com/gin-gonic/gin"
)

// RequestLoggerMiddleware logs each proxied request and records metrics.
// Only records requests destined for downstream services (not gateway internal routes).
func RequestLoggerMiddleware(collector *gateway.MetricsCollector, serviceResolver func(path string) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		serviceName := serviceResolver(c.Request.URL.Path)

		// Capture headers before c.Next() (they may be modified)
		headers := make(map[string]string)
		for key := range c.Request.Header {
			val := c.Request.Header.Get(key)
			// Mask Authorization token for security
			if key == "Authorization" && len(val) > 20 {
				headers[key] = val[:20] + "..."
			} else {
				headers[key] = val
			}
		}

		queryString := c.Request.URL.RawQuery

		c.Next()

		// Skip gateway internal requests (dashboard, metrics, health, etc.)
		if serviceName == "gateway" {
			return
		}

		latency := time.Since(start)
		collector.RecordRequest(
			serviceName,
			c.Request.Method,
			c.Request.URL.Path,
			c.ClientIP(),
			c.Writer.Status(),
			latency,
			headers,
			queryString,
		)
	}
}
