package storage

import (
	"context"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
	"top1000/internal/config"
)

var (
	redisClient *redis.Client
)

// InitRedis 初始化 Redis 连接
func InitRedis() (*RedisStore, error) {
	cfg := config.Get()
	log.Printf("正在连接Redis: %s (DB: %d)", cfg.RedisAddr, cfg.RedisDB)

	redisClient = redis.NewClient(&redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		DialTimeout:  dialTimeout,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		PoolSize:     poolSize,
		MinIdleConns: minIdleConns,
	})

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Printf("Redis连接失败: %v", err)
		return nil, fmt.Errorf("Redis连接失败: %w", err)
	}

	store := NewRedisStore(redisClient)

	log.Println("Redis连接成功")
	return store, nil
}

// CloseRedis 关闭 Redis 连接
func CloseRedis() error {
	if redisClient != nil {
		return redisClient.Close()
	}
	return nil
}
