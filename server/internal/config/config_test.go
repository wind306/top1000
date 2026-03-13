package config

import (
	"os"
	"strconv"
	"sync"
	"testing"
)

// resetConfig 重置配置（仅用于测试）
func resetConfig() {
	initOnce = sync.Once{}
	appConfig.Store((*Config)(nil))
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() func()
		wantErr bool
		check   func(*Config) error
	}{
		{
			name: "从环境变量加载配置",
			setup: func() func() {
				os.Setenv("REDIS_ADDR", "localhost:6379")
				os.Setenv("REDIS_PASSWORD", "password123")
				os.Setenv("REDIS_DB", "2")
				os.Setenv("IYUU_SIGN", "test_sign")
				return func() {
					os.Unsetenv("REDIS_ADDR")
					os.Unsetenv("REDIS_PASSWORD")
					os.Unsetenv("REDIS_DB")
					os.Unsetenv("IYUU_SIGN")
				}
			},
			wantErr: false,
			check: func(cfg *Config) error {
				if cfg.RedisAddr != "localhost:6379" {
					t.Errorf("RedisAddr = %v, want %v", cfg.RedisAddr, "localhost:6379")
				}
				if cfg.RedisPassword != "password123" {
					t.Errorf("RedisPassword = %v, want %v", cfg.RedisPassword, "password123")
				}
				if cfg.RedisDB != 2 {
					t.Errorf("RedisDB = %v, want %v", cfg.RedisDB, 2)
				}
				if cfg.IYYUSign != "test_sign" {
					t.Errorf("IYYUSign = %v, want %v", cfg.IYYUSign, "test_sign")
				}
				return nil
			},
		},
		{
			name: "使用默认值",
			setup: func() func() {
				os.Unsetenv("REDIS_ADDR")
				os.Unsetenv("REDIS_PASSWORD")
				os.Unsetenv("REDIS_DB")
				os.Unsetenv("IYUU_SIGN")
				return func() {}
			},
			wantErr: false,
			check: func(cfg *Config) error {
				if cfg.RedisAddr != "" {
					t.Errorf("RedisAddr = %v, want empty", cfg.RedisAddr)
				}
				if cfg.RedisPassword != "" {
					t.Errorf("RedisPassword = %v, want empty", cfg.RedisPassword)
				}
				if cfg.RedisDB != DefaultRedisDB {
					t.Errorf("RedisDB = %v, want %v", cfg.RedisDB, DefaultRedisDB)
				}
				if cfg.IYYUSign != "" {
					t.Errorf("IYYUSign = %v, want empty", cfg.IYYUSign)
				}
				return nil
			},
		},
		{
			name: "REDIS_DB无效时使用默认值",
			setup: func() func() {
				os.Setenv("REDIS_ADDR", "localhost:6379")
				os.Setenv("REDIS_PASSWORD", "password123")
				os.Setenv("REDIS_DB", "invalid")
				return func() {
					os.Unsetenv("REDIS_ADDR")
					os.Unsetenv("REDIS_PASSWORD")
					os.Unsetenv("REDIS_DB")
				}
			},
			wantErr: false,
			check: func(cfg *Config) error {
				if cfg.RedisDB != DefaultRedisDB {
					t.Errorf("RedisDB = %v, want %v (default)", cfg.RedisDB, DefaultRedisDB)
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetConfig()
			cleanup := tt.setup()
			defer cleanup()

			cfg := Load()
			if err := tt.check(cfg); err != nil {
				t.Errorf("Load() check failed: %v", err)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name       string
		setup      func() func()
		wantErr    bool
		errContains string
	}{
		{
			name: "有效配置",
			setup: func() func() {
				appConfig.Store(&Config{
					RedisAddr:     "localhost:6379",
					RedisPassword: "password123",
					RedisDB:       0,
					IYYUSign:      "",
				})
				return func() { resetConfig() }
			},
			wantErr: false,
		},
		{
			name: "缺少REDIS_ADDR",
			setup: func() func() {
				appConfig.Store(&Config{
					RedisAddr:     "",
					RedisPassword: "password123",
					RedisDB:       0,
				})
				return func() { resetConfig() }
			},
			wantErr:    true,
			errContains: "REDIS_ADDR",
		},
		{
			name: "缺少REDIS_PASSWORD",
			setup: func() func() {
				appConfig.Store(&Config{
					RedisAddr:     "localhost:6379",
					RedisPassword: "",
					RedisDB:       0,
				})
				return func() { resetConfig() }
			},
			wantErr:    true,
			errContains: "REDIS_PASSWORD",
		},
		{
			name: "同时缺少REDIS_ADDR和REDIS_PASSWORD",
			setup: func() func() {
				appConfig.Store(&Config{
					RedisAddr:     "",
					RedisPassword: "",
					RedisDB:       0,
				})
				return func() { resetConfig() }
			},
			wantErr:    true,
			errContains: "REDIS_ADDR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := tt.setup()
			defer cleanup()

			err := Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" {
				if !containsString(err.Error(), tt.errContains) {
					t.Errorf("Validate() error = %v, 期望包含 %v", err, tt.errContains)
				}
			}
		})
	}
}

func TestGet(t *testing.T) {
	t.Run("首次调用返回配置", func(t *testing.T) {
		resetConfig()
		cfg := Get()
		if cfg == nil {
			t.Error("Get() 返回 nil")
		}
	})

	t.Run("多次调用返回相同实例(单例模式)", func(t *testing.T) {
		resetConfig()
		cfg1 := Get()
		cfg2 := Get()
		if cfg1 != cfg2 {
			t.Error("Get() 未遵循单例模式，返回了不同实例")
		}
	})
}

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		setup        func() func()
		want         string
	}{
		{
			name:         "环境变量存在",
			key:          "TEST_VAR",
			defaultValue: "default",
			setup: func() func() {
				os.Setenv("TEST_VAR", "value")
				return func() { os.Unsetenv("TEST_VAR") }
			},
			want: "value",
		},
		{
			name:         "环境变量不存在，返回默认值",
			key:          "TEST_VAR",
			defaultValue: "default",
			setup:        func() func() { return func() {} },
			want:         "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := tt.setup()
			defer cleanup()

			if got := getEnv(tt.key, tt.defaultValue); got != tt.want {
				t.Errorf("getEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGetEnvGeneric 测试泛型环境变量解析函数（Go 1.26）
func TestGetEnvGeneric(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue int
		setup        func() func()
		want         int
	}{
		{
			name:         "环境变量存在且有效",
			key:          "TEST_VAR",
			defaultValue: 0,
			setup: func() func() {
				os.Setenv("TEST_VAR", "5")
				return func() { os.Unsetenv("TEST_VAR") }
			},
			want: 5,
		},
		{
			name:         "环境变量不存在，返回默认值",
			key:          "TEST_VAR",
			defaultValue: 10,
			setup:        func() func() { return func() {} },
			want:         10,
		},
		{
			name:         "环境变量无效，返回默认值",
			key:          "TEST_VAR",
			defaultValue: 10,
			setup: func() func() {
				os.Setenv("TEST_VAR", "invalid")
				return func() { os.Unsetenv("TEST_VAR") }
			},
			want: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := tt.setup()
			defer cleanup()

			// Go 1.26: 使用泛型函数 getEnvGeneric
			parser := func(s string) (int, bool) {
				i, err := strconv.Atoi(s)
				return i, err == nil
			}
			if got := getEnvGeneric(tt.key, tt.defaultValue, parser); got != tt.want {
				t.Errorf("getEnvGeneric() = %v, want %v", got, tt.want)
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && findSubstr(s, substr)
}

func findSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
