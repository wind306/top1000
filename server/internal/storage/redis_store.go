package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
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

	// Redis TTL 特殊返回值
	ttlKeyNotExist = -2 * time.Second // key 不存在（已过期删除）
	ttlKeyNoExpire = -1 * time.Second // key 存在但没有过期时间

	// 时间格式常量
	timeFormat = "2006-01-02 15:04:05" // 数据时间字段格式
)

// RedisStore Redis 实现（DataStore + SitesStore + UpdateLock）
// 组合多个接口，一个实现完成所有功能
type RedisStore struct {
	client *redis.Client

	// Top1000 数据更新锁
	isUpdating   bool
	updateMutex sync.Mutex

	// 站点数据更新锁
	isSitesUpdating   bool
	sitesUpdateMutex sync.Mutex
}

// NewRedisStore 创建 Redis 存储实例
// 返回的实例同时实现 DataStore、SitesStore、UpdateLock 三个接口
func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{client: client}
}

// AsDataStore 将 RedisStore 转换为 DataStore 接口
func (r *RedisStore) AsDataStore() DataStore {
	return r
}

// AsSitesStore 将 RedisStore 转换为 SitesStore 接口
func (r *RedisStore) AsSitesStore() SitesStore {
	return r
}

// AsUpdateLock 将 RedisStore 转换为 UpdateLock 接口
func (r *RedisStore) AsUpdateLock() UpdateLock {
	return r
}

// ===== DataStore 接口实现 =====

// LoadData 加载数据
func (r *RedisStore) LoadData(ctx context.Context) (*model.ProcessedData, error) {
	key := config.DefaultRedisKey

	jsonData, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("%s", errDataNotFound)
		}
		return nil, fmt.Errorf("%s: %w", errRedisReadFailed, err)
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
	if err := r.client.Set(ctx, key, jsonData, 0).Err(); err != nil {
		log.Printf("保存数据到Redis失败: %v", err)
		return fmt.Errorf("%s: %w", errRedisSaveFailed, err)
	}

	log.Printf("数据已保存到Redis（永久存储，过期判断基于数据time字段）")
	return nil
}

// DataExists 检查数据是否存在
func (r *RedisStore) DataExists(ctx context.Context) (bool, error) {
	key := config.DefaultRedisKey

	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("%s: %w", errCheckExistsFailed, err)
	}

	return exists > 0, nil
}

// IsDataExpired 检查数据是否过期
func (r *RedisStore) IsDataExpired(ctx context.Context) (bool, error) {
	// 读取数据
	data, err := r.LoadData(ctx)
	if err != nil {
		return true, nil // 数据不存在或读取失败，认为过期
	}

	// 解析时间字段（API返回的是北京时间UTC+8，需要转换为UTC）
	dataTime, err := time.Parse(timeFormat, data.Time)
	if err != nil {
		log.Printf("解析数据时间失败: %v", err)
		return true, nil // 解析失败，认为过期，强制更新
	}

	// 北京时间是UTC+8，需要减8小时转换为UTC
	dataTime = dataTime.Add(-8 * time.Hour)

	// 计算时间差并判断
	age := time.Since(dataTime)
	isExpired := age > config.DefaultDataExpire

	// 统一日志输出
	logDataStatus(data.Time, age.Round(time.Minute), isExpired, config.DefaultDataExpire)
	return isExpired, nil
}

// ===== SitesStore 接口实现 =====

// LoadSitesData 加载站点数据
func (r *RedisStore) LoadSitesData(ctx context.Context) (any, error) {
	key := config.DefaultSitesKey

	jsonData, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("%s", errSitesNotFound)
		}
		return nil, fmt.Errorf("%s: %w", errRedisReadFailed, err)
	}

	var result any
	if err := json.Unmarshal(jsonData, &result); err != nil {
		return nil, fmt.Errorf("%s: %w", errJSONUnmarshalFailed, err)
	}

	log.Printf("从Redis加载站点数据成功")
	return result, nil
}

// SaveSitesData 保存站点数据
func (r *RedisStore) SaveSitesData(ctx context.Context, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("%s: %w", errJSONMarshalFailed, err)
	}

	key := config.DefaultSitesKey
	// 设置24小时TTL
	ttl := config.DefaultSitesExpire
	if err := r.client.Set(ctx, key, jsonData, ttl).Err(); err != nil {
		log.Printf("保存站点数据到Redis失败: %v", err)
		return fmt.Errorf("%s: %w", errRedisSaveFailed, err)
	}

	log.Printf("站点数据已保存到Redis（TTL: %v）", ttl)
	return nil
}

// SitesDataExists 检查站点数据是否存在
func (r *RedisStore) SitesDataExists(ctx context.Context) (bool, error) {
	key := config.DefaultSitesKey

	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("%s: %w", errCheckExistsFailed, err)
	}

	return exists > 0, nil
}

// ===== UpdateLock 接口实现 =====

// IsUpdating 检查是否正在更新
func (r *RedisStore) IsUpdating() bool {
	r.updateMutex.Lock()
	defer r.updateMutex.Unlock()
	return r.isUpdating
}

// SetUpdating 设置更新标记
func (r *RedisStore) SetUpdating(updating bool) {
	r.updateMutex.Lock()
	defer r.updateMutex.Unlock()
	r.isUpdating = updating
}

// IsSitesUpdating 检查是否正在更新站点数据
func (r *RedisStore) IsSitesUpdating() bool {
	r.sitesUpdateMutex.Lock()
	defer r.sitesUpdateMutex.Unlock()
	return r.isSitesUpdating
}

// SetSitesUpdating 设置站点数据更新标记
func (r *RedisStore) SetSitesUpdating(updating bool) {
	r.sitesUpdateMutex.Lock()
	defer r.sitesUpdateMutex.Unlock()
	r.isSitesUpdating = updating
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
