package middleware

import (
	"github.com/gin-gonic/gin"
)

// ForwardingMiddleware injects X-Forwarded-* headers so downstream services
// can resolve the original client IP and protocol. This ensures middleware like
// auth.go that uses c.ClientIP() will see the real client IP, not the gateway's.
func ForwardingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()

		// Set X-Forwarded-For — append to existing if present
		existing := c.Request.Header.Get("X-Forwarded-For")
		if existing != "" {
			c.Request.Header.Set("X-Forwarded-For", existing+", "+clientIP)
		} else {
			c.Request.Header.Set("X-Forwarded-For", clientIP)
		}

		// Set X-Real-IP to the direct client IP
		c.Request.Header.Set("X-Real-IP", clientIP)

		// Set X-Forwarded-Host
		if c.Request.Host != "" {
			c.Request.Header.Set("X-Forwarded-Host", c.Request.Host)
		}

		// Set X-Forwarded-Proto
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
		if fwdProto := c.Request.Header.Get("X-Forwarded-Proto"); fwdProto != "" {
			scheme = fwdProto
		}
		c.Request.Header.Set("X-Forwarded-Proto", scheme)

		c.Next()
	}
}
