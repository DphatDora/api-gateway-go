package router

import (
	"strings"

	"api-gateway/config"
	"api-gateway/internal/gateway"
	"api-gateway/internal/interface/handler"
	"api-gateway/internal/interface/middleware"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// SetupRoutes configures all gateway routes: CORS, rate limiting, caching, dashboard, and reverse proxy.
func SetupRoutes(
	conf *config.Config,
	proxies []*gateway.ServiceProxy,
	healthChecker *gateway.HealthChecker,
	metricsCollector *gateway.MetricsCollector,
	redisClient *redis.Client,
) *gin.Engine {
	router := gin.Default()

	// Global middleware — order matters:
	// CORS → RateLimit → ForwardingHeaders → Cache → RequestLogger → (proxy handlers)
	router.Use(middleware.CORSMiddleware(conf.App.Whitelist))
	router.Use(middleware.NewRateLimiter(redisClient, conf.RateLimit).Handler())
	router.Use(middleware.ForwardingMiddleware())
	router.Use(middleware.NewResponseCache(redisClient, conf.Cache).Handler())

	// Service resolver: maps request path to service name
	serviceResolver := func(path string) string {
		for _, p := range proxies {
			if strings.HasPrefix(path, p.Prefix) {
				return p.Name
			}
		}
		return "gateway"
	}

	// Request logging middleware (for metrics + dashboard)
	router.Use(middleware.RequestLoggerMiddleware(metricsCollector, serviceResolver))

	// ── Dashboard routes (token-protected) ──
	dashHandler := handler.NewDashboardHandler(healthChecker, metricsCollector)

	if strings.TrimSpace(conf.Log.DashboardToken) != "" {
		dash := router.Group("")
		dash.Use(handler.DashboardAuth(conf))
		dash.GET("/gateway/dashboard", dashHandler.ServeDashboard)

		ctrlHandler := handler.NewServiceControlHandler(conf, healthChecker)

		dashAPI := router.Group("/gateway/api")
		dashAPI.Use(handler.DashboardAuth(conf))
		dashAPI.GET("/services", dashHandler.GetServices)
		dashAPI.GET("/metrics", dashHandler.GetMetrics)
		dashAPI.GET("/requests", dashHandler.GetRequests)
		dashAPI.GET("/logs/files", dashHandler.GetLogFiles)
		dashAPI.GET("/logs", dashHandler.GetLogs)
		dashAPI.POST("/service/:name/control", ctrlHandler.ControlService)
		dashAPI.GET("/db/status", ctrlHandler.GetDBStatus)
	}

	// ── Gateway health endpoint (public) ──
	router.GET("/gateway/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":   "OK",
			"services": healthChecker.GetStatuses(),
		})
	})

	// ── Reverse proxy routes ──
	for _, proxy := range proxies {
		p := proxy // capture loop variable
		router.Any(p.Prefix+"/*proxyPath", func(c *gin.Context) {
			p.ServeHTTP(c.Writer, c.Request)
		})
	}

	return router
}
