package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Go 1.26: 泛型环境变量解析器类型
type envParser[T any] func(string) (T, bool)

// Go 1.26: 通用泛型环境变量解析函数
// 这个SB函数避免了重复代码，DRY原则落地
func getEnvGeneric[T any](key string, defaultValue T, parser envParser[T]) T {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	result, ok := parser(value)
	if !ok {
		return defaultValue
	}
	return result
}

// 默认值常量
const (
	DefaultPort         = "7066"
	DefaultWebDistDir   = "./web-dist"
	DefaultAPIURL       = "https://api.iyuu.cn/top1000.php"
	DefaultDataExpire   = 24 * time.Hour // 数据过期检测阈值
	DefaultRedisDB      = 0              // Redis数据库编号
	DefaultRedisKey     = "top1000:data" // Redis key（Top1000数据）
	DefaultSitesKey     = "top1000:sites" // Redis key（站点数据）
	DefaultSitesExpire  = 24 * time.Hour // 站点数据过期时间
)

// Config 应用程序配置（只保留必须从环境变量读取的配置）
type Config struct {
	RedisAddr          string // Redis地址（必须配置）
	RedisPassword      string // Redis密码（必须配置）
	RedisDB            int    // Redis数据库编号（可选，默认0）
	IYYUSign           string // IYUU签名（可选，用于调用站点API）
	InsecureSkipVerify bool   // 跳过TLS证书验证（可选，仅用于证书过期等异常情况）
}

var (
	appConfig atomic.Value // 存储 *Config
	initOnce  sync.Once
)

// Load 加载配置（单例模式，并发安全）
// Go 1.26: 使用泛型环境变量解析，DRY原则贯彻到底
func Load() *Config {
	initOnce.Do(func() {
		cfg := &Config{
			RedisAddr:          getEnv("REDIS_ADDR", ""),
			RedisPassword:      getEnv("REDIS_PASSWORD", ""),
			// Go 1.26 泛型优化：使用统一的 getEnvGeneric
			RedisDB:            getEnvGeneric("REDIS_DB", DefaultRedisDB, func(s string) (int, bool) {
				i, err := strconv.Atoi(s)
				return i, err == nil
			}),
			IYYUSign: getEnv("IYUU_SIGN", ""),
			InsecureSkipVerify: getEnvGeneric("INSECURE_SKIP_VERIFY", false, func(s string) (bool, bool) {
				// 支持 true/false, 1/0, yes/no
				return s == "true" || s == "1" || s == "yes", true
			}),
		}
		appConfig.Store(cfg)
	})
	return appConfig.Load().(*Config)
}

// ValidationError 配置验证错误（收集所有错误）
type ValidationError struct {
	errors []string
}

// Error 实现 error 接口
func (e *ValidationError) Error() string {
	return fmt.Sprintf("配置验证失败: %s", strings.Join(e.errors, "、"))
}

// Add 添加验证错误
func (e *ValidationError) Add(field string) {
	e.errors = append(e.errors, field)
}

// IsValid 检查是否有错误
func (e *ValidationError) IsValid() bool {
	return len(e.errors) == 0
}

// Validate 验证配置的有效性（返回所有错误）
func Validate() error {
	cfg := Get()
	var errs ValidationError

	if cfg.RedisAddr == "" {
		errs.Add("REDIS_ADDR")
	}
	if cfg.RedisPassword == "" {
		errs.Add("REDIS_PASSWORD")
	}

	if !errs.IsValid() {
		return &errs
	}
	return nil
}

// Get 获取配置实例（并发安全）
func Get() *Config {
	return Load()
}

// getEnv 获取环境变量，如果不存在则返回默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
