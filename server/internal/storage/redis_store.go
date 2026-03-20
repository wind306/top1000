package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"top1000/internal/config"
	"top1000/internal/model"
)

const (
	dialTimeout  = 10 * time.Second
	readTimeout  = 5 * time.Second
	writeTimeout = 5 * time.Second
	poolSize     = 3
	minIdleConns = 1

	// 时间格式常量
	timeFormat           = "2006-01-02 15:04:05" // 数据时间字段格式
	asiaShanghaiTimezone = "Asia/Shanghai"
)

var shanghaiLocation = time.FixedZone(asiaShanghaiTimezone, 8*60*60)

// RedisStore Redis 实现（DataStore + SitesStore + UpdateLock）
// 组合多个接口，一个实现完成所有功能
type RedisStore struct {
	client *redis.Client

	// Top1000 数据更新锁
	isUpdating atomic.Bool

	// 站点数据更新锁
	isSitesUpdating atomic.Bool
}

func (r *RedisStore) readJSONBytes(ctx context.Context, key string, notFoundMsg string) ([]byte, error) {
	jsonData, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("%s", notFoundMsg)
		}
		return nil, fmt.Errorf("%s: %w", errRedisReadFailed, err)
	}

	return jsonData, nil
}

func (r *RedisStore) writeJSONBytes(ctx context.Context, key string, payload []byte, ttl time.Duration, failureLog string, successLog string) error {
	if err := r.client.Set(ctx, key, payload, ttl).Err(); err != nil {
		log.Printf("%s: %v", failureLog, err)
		return fmt.Errorf("%s: %w", errRedisSaveFailed, err)
	}

	log.Printf("%s", successLog)
	return nil
}

func (r *RedisStore) keyExists(ctx context.Context, key string) (bool, error) {
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("%s: %w", errCheckExistsFailed, err)
	}

	return exists > 0, nil
}

// NewRedisStore 创建 Redis 存储实例
// 返回的实例同时实现 DataStore、SitesStore、UpdateLock 三个接口
func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{client: client}
}

// ===== DataStore 接口实现 =====

// LoadData 加载数据
func (r *RedisStore) LoadData(ctx context.Context) (*model.ProcessedData, error) {
	key := config.DefaultRedisKey

	jsonData, err := r.readJSONBytes(ctx, key, errDataNotFound)
	if err != nil {
		return nil, err
	}

	var data model.ProcessedData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("%s: %w", errJSONUnmarshalFailed, err)
	}

	log.Printf("从Redis加载数据成功（共 %d 条记录）", len(data.Items))
	return &data, nil
}

// SaveData 保存数据
func (r *RedisStore) SaveData(ctx context.Context, data model.ProcessedData) error {
	if err := data.Validate(); err != nil {
		log.Printf("数据验证失败，拒绝保存: %v", err)
		return fmt.Errorf("%s: %w", errDataInvalid, err)
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("%s: %w", errJSONMarshalFailed, err)
	}

	key := config.DefaultRedisKey
	// 不设置TTL，数据永久存储
	return r.writeJSONBytes(ctx, key, jsonData, 0, "保存数据到Redis失败", "数据已保存到Redis（永久存储，过期判断基于数据time字段）")
}

// DataExists 检查数据是否存在
func (r *RedisStore) DataExists(ctx context.Context) (bool, error) {
	return r.keyExists(ctx, config.DefaultRedisKey)
}

// IsDataExpired 检查数据是否过期
func (r *RedisStore) IsDataExpired(ctx context.Context) (bool, error) {
	// 读取数据
	data, err := r.LoadData(ctx)
	if err != nil {
		return true, nil // 数据不存在或读取失败，认为过期
	}

	// API 返回的是北京时间，直接按 Asia/Shanghai 解析更明确。
	dataTime, err := time.ParseInLocation(timeFormat, data.Time, shanghaiLocation)
	if err != nil {
		log.Printf("解析数据时间失败: %v", err)
		return true, nil // 解析失败，认为过期，强制更新
	}

	// 计算时间差并判断
	age := time.Since(dataTime)
	isExpired := age > config.DefaultDataExpire

	// 统一日志输出
	logDataStatus(data.Time, age.Round(time.Minute), isExpired, config.DefaultDataExpire)
	return isExpired, nil
}

// ===== SitesStore 接口实现 =====

// LoadSitesData 加载站点数据
func (r *RedisStore) LoadSitesData(ctx context.Context) (json.RawMessage, error) {
	key := config.DefaultSitesKey

	jsonData, err := r.readJSONBytes(ctx, key, errSitesNotFound)
	if err != nil {
		return nil, err
	}

	if !json.Valid(jsonData) {
		return nil, fmt.Errorf("%s: invalid JSON payload", errJSONUnmarshalFailed)
	}

	log.Printf("从Redis加载站点数据成功")
	return json.RawMessage(jsonData), nil
}

// SaveSitesData 保存站点数据
func (r *RedisStore) SaveSitesData(ctx context.Context, data json.RawMessage) error {
	if !json.Valid(data) {
		return fmt.Errorf("%s: %w", errJSONUnmarshalFailed, fmt.Errorf("invalid JSON payload"))
	}

	key := config.DefaultSitesKey
	// 设置24小时TTL
	ttl := config.DefaultSitesExpire
	return r.writeJSONBytes(ctx, key, []byte(data), ttl, "保存站点数据到Redis失败", fmt.Sprintf("站点数据已保存到Redis（TTL: %v）", ttl))
}

// SitesDataExists 检查站点数据是否存在
func (r *RedisStore) SitesDataExists(ctx context.Context) (bool, error) {
	return r.keyExists(ctx, config.DefaultSitesKey)
}

// ===== UpdateLock 接口实现 =====

// IsUpdating 检查是否正在更新
func (r *RedisStore) IsUpdating() bool {
	return r.isUpdating.Load()
}

// SetUpdating 设置更新标记
func (r *RedisStore) SetUpdating(updating bool) {
	r.isUpdating.Store(updating)
}

// IsSitesUpdating 检查是否正在更新站点数据
func (r *RedisStore) IsSitesUpdating() bool {
	return r.isSitesUpdating.Load()
}

// SetSitesUpdating 设置站点数据更新标记
func (r *RedisStore) SetSitesUpdating(updating bool) {
	r.isSitesUpdating.Store(updating)
}

// ===== 辅助函数 =====

// logDataStatus 记录数据状态日志
func logDataStatus(dataTime string, age time.Duration, isExpired bool, threshold time.Duration) {
	if isExpired {
		log.Printf("数据过期了（数据时间: %v, 距今: %v，阈值: %v）", dataTime, age, threshold)
	} else {
		log.Printf("数据还新鲜（数据时间: %v, 距今: %v）", dataTime, age)
	}
}
