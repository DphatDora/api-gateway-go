package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"api-gateway/config"
	"api-gateway/package/logger"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// maxCacheBodySize is the maximum response body size (bytes) eligible for caching (1 MB).
const maxCacheBodySize = 1 << 20

// cachedResponse is the structure persisted in Redis for each cached route.
type cachedResponse struct {
	StatusCode  int    `json:"s"`
	ContentType string `json:"ct"`
	Body        []byte `json:"b"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Response writer wrapper
// ──────────────────────────────────────────────────────────────────────────────

// cacheResponseWriter wraps gin.ResponseWriter to capture the response body
// while still writing through to the original writer (so the client receives it).
type cacheResponseWriter struct {
	gin.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func newCacheResponseWriter(w gin.ResponseWriter) *cacheResponseWriter {
	return &cacheResponseWriter{
		ResponseWriter: w,
		body:           &bytes.Buffer{},
		statusCode:     http.StatusOK,
	}
}

func (w *cacheResponseWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *cacheResponseWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

func (w *cacheResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// ──────────────────────────────────────────────────────────────────────────────
// ResponseCache middleware
// ──────────────────────────────────────────────────────────────────────────────

// ResponseCache caches successful upstream GET responses in Redis.
// Only unauthenticated requests (no Authorization header) are cached, ensuring
// personalised data is never served from cache to the wrong user.
type ResponseCache struct {
	redis *redis.Client
	conf  config.Cache
}

// NewResponseCache creates a ResponseCache. Pass nil redisClient to disable caching.
func NewResponseCache(redisClient *redis.Client, conf config.Cache) *ResponseCache {
	return &ResponseCache{redis: redisClient, conf: conf}
}

// Handler returns a Gin middleware that serves cached responses and populates
// the cache on cache misses.
//
// Caching rules:
//   - Method must be GET.
//   - Request must NOT carry an Authorization header.
//   - Path must match one of the configured cache routes.
//   - Cache-Control: no-cache from the client bypasses the cache.
//   - Only 2xx responses with body ≤ 1 MB are stored.
func (rc *ResponseCache) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rc.conf.Enabled || rc.redis == nil {
			c.Next()
			return
		}

		// Only cache unauthenticated GET requests
		if c.Request.Method != http.MethodGet || c.Request.Header.Get("Authorization") != "" {
			c.Next()
			return
		}

		// Respect explicit cache bypass from client
		if c.Request.Header.Get("Cache-Control") == "no-cache" {
			c.Next()
			return
		}

		// Find matching cacheable route
		path := c.Request.URL.Path
		ttl := 0
		for _, route := range rc.conf.Routes {
			if strings.HasPrefix(path, route.Pattern) {
				ttl = route.TTL
				break
			}
		}
		if ttl <= 0 {
			c.Next()
			return
		}

		cacheKey := generateCacheKey(c.Request)
		ctx := c.Request.Context()

		// ── Cache HIT ────────────────────────────────────────────────────────
		if raw, err := rc.redis.Get(ctx, cacheKey).Bytes(); err == nil {
			var resp cachedResponse
			if err := json.Unmarshal(raw, &resp); err == nil {
				logger.Debugf("[Cache] HIT  path=%s key=%s", path, cacheKey[:12])
				c.Header("X-Cache", "HIT")
				c.Data(resp.StatusCode, resp.ContentType, resp.Body)
				c.Abort()
				return
			}
		}

		// ── Cache MISS: capture the upstream response ─────────────────────────
		// Set X-Cache: MISS before WriteHeader so the header is sent to the client.
		c.Header("X-Cache", "MISS")
		crw := newCacheResponseWriter(c.Writer)
		c.Writer = crw

		c.Next()

		// Store in Redis only for successful, reasonably-sized responses.
		status := crw.statusCode
		if status < 200 || status >= 300 || crw.body.Len() == 0 || crw.body.Len() > maxCacheBodySize {
			return
		}

		resp := cachedResponse{
			StatusCode:  status,
			ContentType: crw.ResponseWriter.Header().Get("Content-Type"),
			Body:        crw.body.Bytes(),
		}
		data, err := json.Marshal(resp)
		if err != nil {
			return
		}
		if err := rc.redis.Set(ctx, cacheKey, data, time.Duration(ttl)*time.Second).Err(); err != nil {
			logger.Warnf("[Cache] Failed to store key=%s: %v", cacheKey[:12], err)
		} else {
			logger.Debugf("[Cache] STORED path=%s ttl=%ds", path, ttl)
		}
	}
}

// generateCacheKey builds a stable, collision-resistant cache key by hashing
// the HTTP method, path, and canonically sorted query parameters.
func generateCacheKey(r *http.Request) string {
	q := r.URL.Query()
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		vals := q[k]
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, k+"="+v)
		}
	}

	canonical := r.Method + ":" + r.URL.Path + "?" + strings.Join(parts, "&")
	h := sha256.Sum256([]byte(canonical))
	return fmt.Sprintf("gw:cache:%s", hex.EncodeToString(h[:]))
}
