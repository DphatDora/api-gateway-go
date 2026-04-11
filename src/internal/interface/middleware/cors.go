package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func CORSMiddleware(whitelist []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		allowed := false
		if len(whitelist) == 0 {
			allowed = true
			origin = "*"
		} else {
			for _, allowedOrigin := range whitelist {
				if strings.TrimSpace(allowedOrigin) == origin {
					allowed = true
					break
				}
			}
		}

		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, Authorization, X-Requested-With")
			c.Header("Access-Control-Expose-Headers", "Content-Length, Authorization")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Max-Age", "86400")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
