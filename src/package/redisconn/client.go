package redisconn

import (
	"context"
	"fmt"
	"time"

	"api-gateway/config"

	"github.com/redis/go-redis/v9"
)

func NewClient(conf *config.Redis) (*redis.Client, error) {
	if conf.Host == "" {
		return nil, nil
	}

	poolSize := conf.PoolSize
	if poolSize <= 0 {
		poolSize = 10
	}

	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%s", conf.Host, conf.Port),
		Password:     conf.Password,
		DB:           conf.DB,
		PoolSize:     poolSize,
		MinIdleConns: 2,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping failed at %s:%s: %w", conf.Host, conf.Port, err)
	}

	return client, nil
}
