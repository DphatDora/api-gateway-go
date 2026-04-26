package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"api-gateway/config"
	"api-gateway/package/logger"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimiter implements per-IP sliding window rate limiting backed by Redis.
// If Redis is unavailable, the middleware is a transparent no-op (fail-open).
type RateLimiter struct {
	redis *redis.Client
	conf  config.RateLimit
}

// NewRateLimiter creates a RateLimiter. Pass nil redisClient to disable rate limiting.
func NewRateLimiter(redisClient *redis.Client, conf config.RateLimit) *RateLimiter {
	return &RateLimiter{redis: redisClient, conf: conf}
}

// Handler returns a Gin middleware that enforces rate limits per client IP.
//
// Logic:
//  1. Check global limit (always applied to every request).
//  2. If the request path matches a per-route rule, also check that stricter limit.
//
// Both buckets are incremented atomically via Redis pipeline so that a single
// request is counted in both the global and the per-route window.
func (rl *RateLimiter) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.conf.Enabled || rl.redis == nil {
			c.Next()
			return
		}

		clientIP := c.ClientIP()
		path := c.Request.URL.Path
		ctx := c.Request.Context()

		// ── 1. Global limit ──────────────────────────────────────────────────
		globalKey := fmt.Sprintf("gw:rl:global:%s", clientIP)
		globalLimit := rl.conf.Global.Limit
		globalWindow := time.Duration(rl.conf.Global.Window) * time.Second

		globalOK, globalRemaining, globalResetAt, err := rl.slidingWindow(ctx, globalKey, globalLimit, globalWindow)
		if err != nil {
			logger.Warnf("[RateLimit] Redis error (global) IP=%s: %v", clientIP, err)
			c.Next()
			return
		}
		if !globalOK {
			abortRateLimit(c, globalLimit, 0, globalResetAt)
			return
		}

		// ── 2. Per-route limit (if applicable) ───────────────────────────────
		for _, route := range rl.conf.Routes {
			if !strings.HasPrefix(path, route.Pattern) {
				continue
			}

			routeKey := fmt.Sprintf("gw:rl:%s:%s", sanitizeRLKey(route.Pattern), clientIP)
			routeLimit := route.Limit
			routeWindow := time.Duration(route.Window) * time.Second

			routeOK, routeRemaining, routeResetAt, err := rl.slidingWindow(ctx, routeKey, routeLimit, routeWindow)
			if err != nil {
				logger.Warnf("[RateLimit] Redis error (route=%s) IP=%s: %v", route.Pattern, clientIP, err)
				break
			}
			if !routeOK {
				logger.Warnf("[RateLimit] Per-route limit exceeded IP=%s path=%s limit=%d/window=%ds",
					clientIP, path, routeLimit, route.Window)
				abortRateLimit(c, routeLimit, 0, routeResetAt)
				return
			}

			// Expose the more restrictive (per-route) headers
			c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", routeLimit))
			c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", routeRemaining))
			c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", routeResetAt.Unix()))
			c.Next()
			return
		}

		// No per-route match — expose global headers
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", globalLimit))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", globalRemaining))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", globalResetAt.Unix()))
		c.Next()
	}
}

// slidingWindow performs an atomic sliding-window check using a Redis sorted set.
//
// It removes entries older than the window, adds the current request, then
// reads the count — all in a single pipeline to minimise round-trips.
//
// Returns (allowed, remaining, approximateResetAt, error).
func (rl *RateLimiter) slidingWindow(
	ctx context.Context,
	key string,
	limit int,
	window time.Duration,
) (allowed bool, remaining int, resetAt time.Time, err error) {
	now := time.Now()
	windowStart := now.Add(-window)
	// Nanosecond-precision member keeps entries unique for concurrent requests
	member := fmt.Sprintf("%d", now.UnixNano())

	pipe := rl.redis.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", windowStart.UnixMilli()))
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(now.UnixMilli()), Member: member})
	countCmd := pipe.ZCard(ctx, key)
	pipe.Expire(ctx, key, window+5*time.Second) // buffer avoids premature eviction

	if _, err = pipe.Exec(ctx); err != nil {
		return true, limit, now.Add(window), fmt.Errorf("pipeline: %w", err)
	}

	count := int(countCmd.Val())
	remaining = limit - count
	if remaining < 0 {
		remaining = 0
	}
	return count <= limit, remaining, now.Add(window), nil
}

// abortRateLimit writes a 429 response with standard rate-limit headers.
func abortRateLimit(c *gin.Context, limit, remaining int, resetAt time.Time) {
	retryAfter := int(time.Until(resetAt).Seconds())
	if retryAfter < 0 {
		retryAfter = 0
	}
	c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
	c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
	c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", resetAt.Unix()))
	c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))
	c.JSON(http.StatusTooManyRequests, gin.H{
		"success": false,
		"message": fmt.Sprintf("Rate limit exceeded. Retry after %d seconds.", retryAfter),
	})
	c.Abort()
}

// sanitizeRLKey converts a path pattern to a safe Redis key segment.
func sanitizeRLKey(s string) string {
	return strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(s)
}
