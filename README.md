# API Gateway — Go

A reverse proxy gateway built with Go and Gin that serves as a single entry point for multiple backend services with health checking, request metrics, and real-time monitoring dashboard.

## Overview

Routes requests to downstream services:

- **wetalk-academy** (:8046)
- **social-platform** (:8045)
- ...

**Features**: Header forwarding, automatic health checks (60s interval), request metrics, CORS support, embedded monitoring dashboard.

## Project Structure

```
src/
├── cmd/server/main.go        # Entry point
├── config/                   # Configuration (viper + godotenv)
├── internal/
│   ├── gateway/              # Proxy, health checker, metrics
│   └── interface/            # Handlers, middleware, routes
├── package/logger/           # Structured logging
├── web/dashboard/            # Dashboard UI (embedded)
└── logs/                     # Log files
```

## Quick Start

```bash
cd api-gateway-go/src
cp .env.example .env          # Configure if needed
go run ./cmd/server/main.go
```

Access dashboard: `http://localhost:8080/gateway/dashboard?token=your_token`

## Routing

| Path                 | Target                            |
| -------------------- | --------------------------------- |
| `/academy/**`        | `http://localhost:8046/api/v1/**` |
| `/social/**`         | `http://localhost:8045/api/v1/**` |
| `/gateway/dashboard` | Monitoring UI (token-protected)   |
| `/gateway/api/*`     | Dashboard APIs                    |
| `/gateway/health`    | Service health status             |

## Key Features

- **Header Forwarding**: Authorization, X-Forwarded-\*, X-Real-IP
- **Health Checking**: GET `/api/v1/health` every 10 seconds
- **Metrics**: Per-service (count, latency, status codes), system (goroutines, memory, uptime)
- **Dashboard**: Service map, request monitor, log viewer, theme toggle
- **CORS**: Configurable origin whitelist
- **Logging**: Structured JSON logs with rotation

## Configuration

### Environment Variables

```bash
PORT=8080
LOG_LEVEL=info
LOG_DASHBOARD_TOKEN=your_token
```

### config.yaml

```yaml
app:
  port: 8080
  whitelist:
    - http://localhost:3000

services:
  - name: wetalk-academy
    prefix: /academy
    target: http://localhost:8046/api/v1
    healthPath: /api/v1/health
```

## API Endpoints

```bash
GET /gateway/health                    # Status (public)
GET /gateway/dashboard                 # Dashboard UI (token-protected)
GET /gateway/api/services              # Service statuses
GET /gateway/api/metrics               # System & service metrics
GET /gateway/api/requests              # Recent requests
GET /gateway/api/logs?lines=200        # Application logs
```

## Next Steps

1. Update CORS on downstream services to accept gateway only
2. Point frontend API calls to `http://localhost:8080`
3. Add more services in `config.yaml`
4. Deploy with environment-based configuration

## License

Internal project for KLTN study
