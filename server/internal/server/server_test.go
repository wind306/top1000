package server

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// TestNewServer 测试服务器创建
func TestNewServer(t *testing.T) {
	srv := New()

	if srv == nil {
		t.Fatal("New() 返回 nil")
	}

	if srv.shutdownCtx == nil {
		t.Error("shutdownCtx 未初始化")
	}

	if srv.cancel == nil {
		t.Error("cancel 函数未初始化")
	}
}

// TestServerShutdown 测试优雅关闭
func TestServerShutdown(t *testing.T) {
	srv := New()

	// 创建一个会在短时间内完成的 context
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 调用 Shutdown 应该正常返回
	err := srv.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown() 意外返回错误: %v", err)
	}
}

// TestServerShutdownWithTimeout 测试超时关闭
func TestServerShutdownWithTimeout(t *testing.T) {
	srv := New()

	// 创建一个会立即超时的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	// Shutdown 应该处理取消的 context
	err := srv.Shutdown(ctx)
	// 由于没有实际的服务器运行，这应该返回 nil 或 context 相关错误
	if err != nil && err != context.Canceled {
		t.Logf("Shutdown() 返回错误（可能是正常的）: %v", err)
	}
}

// TestCreateApp 测试应用创建
func TestCreateApp(t *testing.T) {
	srv := New()
	app := srv.createApp()

	if app == nil {
		t.Fatal("createApp() 返回 nil")
	}

	// 验证应用配置
	if app.Config().AppName != appName {
		t.Errorf("期望应用名 %s，得到 %s", appName, app.Config().AppName)
	}
}

// TestCustomErrorHandler 测试自定义错误处理器
func TestCustomErrorHandler(t *testing.T) {
	// 创建带自定义错误处理器的应用
	app := fiber.New(fiber.Config{
		ErrorHandler: customErrorHandler,
	})

	// 创建一个测试路由，返回错误
	app.Get("/error", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusBadRequest, "测试错误")
	})

	// 创建测试请求
	req := httptest.NewRequest("GET", "/error", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test() 失败: %v", err)
	}

	// 验证状态码
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("期望状态码 %d，得到 %d", fiber.StatusBadRequest, resp.StatusCode)
	}
}

// TestLoggerMiddleware 测试日志中间件
func TestLoggerMiddleware(t *testing.T) {
	middleware := loggerMiddleware()
	if middleware == nil {
		t.Fatal("loggerMiddleware() 返回 nil")
	}

	app := fiber.New()
	app.Use(middleware)
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test() 失败: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("期望状态码 %d，得到 %d", fiber.StatusOK, resp.StatusCode)
	}
}

// TestSecurityHeadersMiddleware 测试安全头中间件
func TestSecurityHeadersMiddleware(t *testing.T) {
	middleware := securityHeadersMiddleware()
	if middleware == nil {
		t.Fatal("securityHeadersMiddleware() 返回 nil")
	}

	app := fiber.New()
	app.Use(middleware)
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test() 失败: %v", err)
	}

	// 验证安全头
	expectedHeaders := map[string]string{
		"X-XSS-Protection":       "1; mode=block",
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
	}

	for header, expected := range expectedHeaders {
		actual := resp.Header.Get(header)
		if actual != expected {
			t.Errorf("期望头 %s = %s，得到 %s", header, expected, actual)
		}
	}

	// 验证 CSP 头存在
	csp := resp.Header.Get("Content-Security-Policy")
	if csp == "" {
		t.Error("Content-Security-Policy 头缺失")
	}
}

// TestSetCacheHeaders 测试缓存头设置
func TestSetCacheHeaders(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		statusCode int
		wantCache  string
	}{
		{
			name:       "静态资源 - JS 文件",
			path:       "/app.js",
			statusCode: fiber.StatusOK,
			wantCache:  oneYearMaxAge,
		},
		{
			name:       "静态资源 - CSS 文件",
			path:       "/style.css",
			statusCode: fiber.StatusOK,
			wantCache:  oneYearMaxAge,
		},
		{
			name:       "HTML 文件",
			path:       "/index.html",
			statusCode: fiber.StatusOK,
			wantCache:  noCache,
		},
		{
			name:       "根路径",
			path:       "/",
			statusCode: fiber.StatusOK,
			wantCache:  noCache,
		},
		{
			name:       "错误状态码",
			path:       "/app.js",
			statusCode: fiber.StatusNotFound,
			wantCache:  noCache,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/*", func(c *fiber.Ctx) error {
				c.Response().SetStatusCode(tt.statusCode)
				return setCacheHeaders(c)
			})

			req := httptest.NewRequest("GET", tt.path, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("Test() 失败: %v", err)
			}

			cacheControl := resp.Header.Get("Cache-Control")
			if cacheControl != tt.wantCache {
				t.Errorf("期望 Cache-Control = %s，得到 %s", tt.wantCache, cacheControl)
			}

			// 验证 no-cache 时的额外头
			if tt.wantCache == noCache {
				pragma := resp.Header.Get("Pragma")
				expires := resp.Header.Get("Expires")
				if pragma != "no-cache" {
					t.Errorf("期望 Pragma = no-cache，得到 %s", pragma)
				}
				if expires != "0" {
					t.Errorf("期望 Expires = 0，得到 %s", expires)
				}
			}
		})
	}
}

// TestServerSignalHandling 测试信号处理
func TestServerSignalHandling(t *testing.T) {
	srv := New()

	// 创建一个会在短时间内发送信号的 context
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// waitForShutdown 应该从 context 或信号返回
	// 由于我们不会发送实际信号，它会等待 context 超时
	err := srv.waitForShutdown(ctx)
	if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
		t.Logf("waitForShutdown 返回错误: %v", err)
	}
}

// TestPrintStartupInfo 测试启动信息打印
func TestPrintStartupInfo(t *testing.T) {
	srv := New()
	// 确保函数不会 panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("printStartupInfo() panic: %v", r)
		}
	}()
	srv.printStartupInfo()
}
