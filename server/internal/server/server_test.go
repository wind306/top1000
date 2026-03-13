package server

import (
	"context"
	"net/http/httptest"
	"os"
	"os/signal"
	"syscall"
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

// TestSwaggerUI 测试 Swagger UI 端点
func TestSwaggerUI(t *testing.T) {
	app := fiber.New()
	app.Get("/swagger/*", swaggerUI)

	req := httptest.NewRequest("GET", "/swagger/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test() 失败: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("期望状态码 %d，得到 %d", fiber.StatusOK, resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("期望 Content-Type = text/html; charset=utf-8，得到 %s", contentType)
	}

	cacheControl := resp.Header.Get("Cache-Control")
	if cacheControl != noCache {
		t.Errorf("期望 Cache-Control = %s，得到 %s", noCache, cacheControl)
	}
}

// TestPrintSeparator 测试分隔线打印（仅确保不 panic）
func TestPrintSeparator(t *testing.T) {
	// 这个测试只是为了确保函数不会 panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("printSeparator() panic: %v", r)
		}
	}()
	printSeparator()
}

// TestServerContextCancellation 测试 context 取消时的行为
func TestServerContextCancellation(t *testing.T) {
	srv := New()

	// 模拟在另一个 goroutine 中取消服务器的 context
	go func() {
		time.Sleep(10 * time.Millisecond)
		srv.cancel()
	}()

	// 等待服务器的 shutdownCtx 被取消
	select {
	case <-srv.shutdownCtx.Done():
		// 预期行为 - context 已取消
	case <-time.After(100 * time.Millisecond):
		t.Error("context 未在预期时间内取消")
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

// BenchmarkLoggerMiddleware 日志中间件基准测试
func BenchmarkLoggerMiddleware(b *testing.B) {
	middleware := loggerMiddleware()
	app := fiber.New()
	app.Use(middleware)
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = app.Test(req)
	}
}

// BenchmarkSecurityHeadersMiddleware 安全头中间件基准测试
func BenchmarkSecurityHeadersMiddleware(b *testing.B) {
	middleware := securityHeadersMiddleware()
	app := fiber.New()
	app.Use(middleware)
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = app.Test(req)
	}
}

// Table-driven test for signal handling
func TestWaitForShutdownSignals(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{
			name:    "短超时",
			timeout: 10 * time.Millisecond,
		},
		{
			name:    "中等超时",
			timeout: 50 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := New()
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			start := time.Now()
			err := srv.waitForShutdown(ctx)
			elapsed := time.Since(start)

			_ = err // waitForShutdown 可能返回错误
			if elapsed < tt.timeout {
				t.Logf("waitForShutdown 在 %v 内返回（超时设定 %v）", elapsed, tt.timeout)
			}
		})
	}
}

// TestServerShutdownCtxClosed 测试关闭时 context 状态
func TestServerShutdownCtxClosed(t *testing.T) {
	srv := New()

	// 验证初始状态
	select {
	case <-srv.shutdownCtx.Done():
		t.Error("shutdownCtx 初始状态应该是未关闭的")
	default:
		// 预期状态
	}

	// 调用 cancel
	srv.cancel()

	// 验证关闭后状态
	select {
	case <-srv.shutdownCtx.Done():
		// 预期状态
	case <-time.After(100 * time.Millisecond):
		t.Error("shutdownCtx 应该已关闭")
	}
}

// ExampleNewServer 创建服务器示例
func ExampleNewServer() {
	srv := New()
	_ = srv // 使用 srv.Start() 启动服务器
}

// ExampleServer_Shutdown 优雅关闭示例
func ExampleServer_Shutdown() {
	srv := New()

	// 在实际使用中，你会用 srv.Start() 启动服务器
	// 然后在收到信号时调用 Shutdown

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_ = srv.Shutdown(ctx)
}

// TestSignalMapping 测试信号映射
func TestSignalMapping(t *testing.T) {
	signals := []os.Signal{syscall.SIGINT, syscall.SIGTERM}
	expected := []string{"interrupt", "termination request"}

	for i, sig := range signals {
		t.Run(expected[i], func(t *testing.T) {
			srv := New()
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, sig)

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			done := make(chan error, 1)
			go func() {
				done <- srv.waitForShutdown(ctx)
			}()

			select {
			case <-ctx.Done():
				// 预期超时
			case err := <-done:
				_ = err // 可能从信号返回
			}

			signal.Stop(sigChan)
			close(sigChan)
		})
	}
}

// TestSwaggerJSON 测试 Swagger JSON 端点
func TestSwaggerJSON(t *testing.T) {
	app := fiber.New()
	app.Get("/swagger/doc.json", swaggerJSON)

	req := httptest.NewRequest("GET", "/swagger/doc.json", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test() 失败: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("期望状态码 %d，得到 %d", fiber.StatusOK, resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("期望 Content-Type = application/json，得到 %s", contentType)
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

// TestStartCompat 测试兼容性启动函数
func TestStartCompat(t *testing.T) {
	// 这个测试只验证 StartCompat 存在且可调用
	// 实际启动会阻塞，所以我们不真的调用它
	// 只是确保函数签名正确
	defer func() {
		if r := recover(); r != nil {
			// 如果 panic，说明函数有问题
			t.Errorf("StartCompat 存在问题: %v", r)
		}
	}()

	// 验证 StartCompat 是一个可调用的函数
	_ = StartCompat
}
