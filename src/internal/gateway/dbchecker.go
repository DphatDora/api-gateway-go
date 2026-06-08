package gateway

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	// PostgreSQL driver
	_ "github.com/lib/pq"

	// MongoDB driver v1
	"go.mongodb.org/mongo-driver/mongo"
	mongoopts "go.mongodb.org/mongo-driver/mongo/options"

	// Redis
	"github.com/redis/go-redis/v9"
)

// DBStatus holds the connection status for a single database.
type DBStatus struct {
	Type       string `json:"type"`
	Name       string `json:"name"`
	Status     string `json:"status"` // "UP", "DOWN", "UNCONFIGURED"
	Latency    int64  `json:"latency_ms"`
	Error      string `json:"error,omitempty"`
	DisplayURL string `json:"display_url,omitempty"`
}

// CheckPostgres tries to connect + ping a Postgres DSN.
func CheckPostgres(dsn string) DBStatus {
	s := DBStatus{Type: "postgres", Name: "PostgreSQL"}
	if strings.TrimSpace(dsn) == "" {
		s.Status = "UNCONFIGURED"
		return s
	}
	s.DisplayURL = maskConnectionString(dsn)
	start := time.Now()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		s.Status = "DOWN"
		s.Error = err.Error()
		return s
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		s.Status = "DOWN"
		s.Error = err.Error()
		return s
	}
	s.Status = "UP"
	s.Latency = time.Since(start).Milliseconds()
	return s
}

// CheckMongo tries to connect + ping a MongoDB URI.
func CheckMongo(uri string) DBStatus {
	s := DBStatus{Type: "mongo", Name: "MongoDB"}
	if strings.TrimSpace(uri) == "" {
		s.Status = "UNCONFIGURED"
		return s
	}
	s.DisplayURL = maskConnectionString(uri)
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, mongoopts.Client().ApplyURI(uri))
	if err != nil {
		s.Status = "DOWN"
		s.Error = err.Error()
		return s
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, nil); err != nil {
		s.Status = "DOWN"
		s.Error = err.Error()
		return s
	}
	s.Status = "UP"
	s.Latency = time.Since(start).Milliseconds()
	return s
}

// CheckRedisDB pings a Redis URL (redis://:password@host:port/db).
func CheckRedisDB(rawURL string) DBStatus {
	s := DBStatus{Type: "redis", Name: "Redis"}
	if strings.TrimSpace(rawURL) == "" {
		s.Status = "UNCONFIGURED"
		return s
	}
	s.DisplayURL = maskConnectionString(rawURL)
	opt, err := parseRedisURL(rawURL)
	if err != nil {
		s.Status = "DOWN"
		s.Error = fmt.Sprintf("invalid URL: %v", err)
		return s
	}
	start := time.Now()
	client := redis.NewClient(opt)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		s.Status = "DOWN"
		s.Error = err.Error()
		return s
	}
	s.Status = "UP"
	s.Latency = time.Since(start).Milliseconds()
	return s
}

func maskConnectionString(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.User != nil {
		username := u.User.Username()
		if _, hasPassword := u.User.Password(); hasPassword {
			u.User = url.UserPassword(username, "redacted")
		}
		return u.String()
	}

	parts := strings.Fields(raw)
	for i, part := range parts {
		if strings.HasPrefix(strings.ToLower(part), "password=") {
			parts[i] = "password=redacted"
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	return raw
}

func parseRedisURL(rawURL string) (*redis.Options, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	opts := &redis.Options{Addr: u.Host}
	if u.User != nil {
		opts.Password, _ = u.User.Password()
	}
	return opts, nil
}
