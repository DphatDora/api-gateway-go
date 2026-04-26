package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"api-gateway/config"
	"api-gateway/internal/gateway"
	"api-gateway/internal/interface/router"
	"api-gateway/package/logger"
	"api-gateway/package/redisconn"

	"github.com/redis/go-redis/v9"
)

const (
	DefaultPort = 8080
)

func main() {
	time.Local = time.UTC

	conf := config.GetConfig()
	if err := logger.Init(&conf.Log); err != nil {
		fmt.Fprintf(os.Stderr, "logger init: %v\n", err)
		os.Exit(1)
	}
	if err := logger.InitRequestLog(&conf.Log); err != nil {
		fmt.Fprintf(os.Stderr, "request logger init: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	logger.Debugf("[DEBUG] Config: %+v", conf)

	// Build reverse proxies for each service
	var proxies []*gateway.ServiceProxy
	for _, svc := range conf.Services {
		proxy, err := gateway.NewServiceProxy(svc)
		if err != nil {
			logger.Fatalf("[FATAL] Failed to create proxy for %s: %v", svc.Name, err)
		}
		proxies = append(proxies, proxy)
		logger.Infof("[Gateway] Registered service: %s (%s -> %s)", svc.Name, svc.Prefix, svc.Target)
	}

	// Health checker
	healthChecker := gateway.NewHealthChecker(conf.Services)
	healthChecker.Start(60 * time.Second)
	defer healthChecker.Stop()

	// Metrics collector
	metricsCollector := gateway.NewMetricsCollector()
	metricsCollector.LoadFromLog()

	// Init Redis (optional — rate limiting and caching are disabled if Redis is unavailable)
	var redisClient *redis.Client
	rc, err := redisconn.NewClient(&conf.Redis)
	if err != nil {
		if conf.Redis.Required {
			logger.Fatalf("[FATAL] Redis is required but unavailable: %v", err)
		}
		logger.Warnf("[Gateway] Redis unavailable: %v. Rate limiting and caching disabled.", err)
	} else {
		redisClient = rc
		if redisClient != nil {
			logger.Infof("[Gateway] Redis connected at %s:%s", conf.Redis.Host, conf.Redis.Port)
			defer func() { _ = redisClient.Close() }()
		}
	}

	// Setup routes
	r := router.SetupRoutes(&conf, proxies, healthChecker, metricsCollector, redisClient)

	port := conf.App.Port
	if port == 0 {
		port = DefaultPort
	}

	logger.Infof("[Gateway] Starting on PORT %d", port)
	logger.Infof("[Gateway] Dashboard: http://%s:%d/gateway/dashboard", conf.App.Host, port)

	if err := r.Run(":" + strconv.Itoa(port)); err != nil {
		logger.Fatalf("[FATAL] Server failed: %v", err)
	}
}
