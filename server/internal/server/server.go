package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"top1000/internal/api"
	"top1000/internal/config"
	"top1000/internal/crawler"
	"top1000/internal/storage"

	docs "top1000/docs" // Swagger docs

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

const (
	appName          = "Top1000"
	requestBodyLimit = 4 * 1024 * 1024
	oneYearMaxAge    = "public, max-age=31536000"
	noCache          = "no-cache, no-store, must-revalidate"
	separatorLength  = 40
	// 默认优雅关闭超时时间
	defaultShutdownTimeout = 30 * time.Second
)

// Server 服务器结构体（保持状态，方便测试和优雅关闭）
type Server struct {
	app         *fiber.App
	cfg         *config.Config
	shutdownCtx context.Context
	cancel      context.CancelFunc
}

// New 创建一个新的服务器实例（不自动启动）
func New() *Server {
	cfg := config.Get()

	// 创建可取消的 context 用于优雅关闭
	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		cfg:         cfg,
		shutdownCtx: ctx,
		cancel:      cancel,
	}
}

// Start 启动Web服务器（支持优雅关闭）
// 这个方法会阻塞直到服务关闭或发生错误
func (s *Server) Start(ctx context.Context) error {
	// 验证配置
	if err := config.Validate(); err != nil {
		return fmt.Errorf("配置验证失败: %w", err)
	}

	printSeparator()
	log.Println("   Top1000 服务正在启动...")
	printSeparator()

	// 初始化存储
	if err := s.initStorage(); err != nil {
		return fmt.Errorf("Redis初始化失败: %w", err)
	}

	// 创建应用
	s.app = s.createApp()

	// 预加载数据
	s.preloadData()

	// 打印启动信息
	s.printStartupInfo()

	// 设置信号监听
	shutdownErr := make(chan error, 1)
	go func() {
		shutdownErr <- s.waitForShutdown(ctx)
	}()

	// 启动服务器（非阻塞方式）
	go func() {
		if err := s.app.Listen(":" + config.DefaultPort); err != nil && !errors.Is(err, http.ErrServerClosed) {
			shutdownErr <- fmt.Errorf("服务启动失败: %w", err)
		}
	}()

	// 等待关闭信号或启动错误
	select {
	case err := <-shutdownErr:
		// 优雅关闭
		s.shutdown()
		return err
	case <-s.shutdownCtx.Done():
		// 通过 cancel 触发的关闭
		s.shutdown()
		return nil
	}
}

// Shutdown 主动关闭服务器
func (s *Server) Shutdown(ctx context.Context) error {
	s.cancel()
	return s.shutdownWithTimeout(ctx)
}

// shutdownWithTimeout 带超时的关闭
func (s *Server) shutdownWithTimeout(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, defaultShutdownTimeout)
	defer cancel()

	// 关闭 HTTP 服务器
	if s.app != nil {
		if err := s.app.ShutdownWithContext(shutdownCtx); err != nil {
			log.Printf("服务关闭失败: %v", err)
			return err
		}
	}

	// 关闭 Redis 连接
	s.closeRedis()

	log.Println("服务已安全关闭")
	return nil
}

// shutdown 内部关闭方法（无超时控制）
func (s *Server) shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
	defer cancel()

	if err := s.shutdownWithTimeout(ctx); err != nil {
		log.Printf("关闭过程中发生错误: %v", err)
	}
}

// waitForShutdown 等待系统信号
func (s *Server) waitForShutdown(ctx context.Context) error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		log.Printf("收到信号 %v，正在优雅关闭...", sig)
		return nil
	case <-ctx.Done():
		log.Println("收到取消请求，正在关闭...")
		return ctx.Err()
	}
}

// createApp 创建Fiber应用并配置中间件和路由
func (s *Server) createApp() *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      appName,
		StrictRouting: true,
		BodyLimit:    requestBodyLimit,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		// 错误处理自定义
		ErrorHandler: customErrorHandler,
	})

	s.setupMiddleware(app)
	s.setupRoutes(app)

	return app
}

// customErrorHandler 自定义错误处理器
func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}

	// 记录错误日志
	log.Printf("请求错误: %s %s - %d - %v",
		c.Method(),
		c.Path(),
		code,
		err,
	)

	// 返回 JSON 错误响应
	return c.Status(code).JSON(fiber.Map{
		"error": err.Error(),
	})
}

// setupMiddleware 配置中间件
func (s *Server) setupMiddleware(app *fiber.App) {
	app.Use(recover.New())
	app.Use(loggerMiddleware())
	app.Use(securityHeadersMiddleware())
	app.Use(compress.New())
}

// loggerMiddleware 日志中间件
func loggerMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		log.Printf("[%s] %s %s - %d - %v",
			time.Now().Format("2006-01-02 15:04:05"),
			c.Method(),
			c.Path(),
			c.Response().StatusCode(),
			time.Since(start),
		)
		return err
	}
}

// securityHeadersMiddleware 安全响应头中间件
func securityHeadersMiddleware() fiber.Handler {
	cspHeader := "default-src 'self'; " +
		"script-src 'self' 'unsafe-inline' 'unsafe-eval' https://log.939593.xyz; " +
		"img-src 'self' data: https: https://lsky.939593.xyz:11111; " +
		"style-src 'self' 'unsafe-inline'; " +
		"connect-src 'self' https://log.939593.xyz;"

	return func(c *fiber.Ctx) error {
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("Content-Security-Policy", cspHeader)
		return c.Next()
	}
}

// setupRoutes 配置路由
func (s *Server) setupRoutes(app *fiber.App) {
	handler := api.NewHandler(
		storage.GetDefaultStore(),
		storage.GetDefaultSitesStore(),
		storage.GetDefaultLock(),
	)

	handler.RegisterRoutes(app)

	app.Get("/swagger/*", swaggerUI)
	app.Get("/swagger/doc.json", swaggerJSON)

	app.Static("/", config.DefaultWebDistDir, fiber.Static{
		CacheDuration:  0,
		Browse:         true,
		MaxAge:         0,
		ModifyResponse: setCacheHeaders,
	})
}

// swaggerUI 返回 Swagger UI HTML（使用模板）
func swaggerUI(c *fiber.Ctx) error {
	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <title>Top1000 API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui.css">
    <style>
        html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin: 0; padding: 0; font-family: "Helvetica Neue", Helvetica, Arial, sans-serif; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui-bundle.js" charset="UTF-8"></script>
    <script src="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui-standalone-preset.js" charset="UTF-8"></script>
    <script>
        window.onload = function() {
            const ui = SwaggerUIBundle({
                url: "/swagger/doc.json",
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout"
            })
            window.ui = ui
        }
    </script>
</body>
</html>`
	c.Set("Content-Type", "text/html; charset=utf-8")
	c.Set("Cache-Control", noCache)
	return c.Send([]byte(html))
}

// swaggerJSON 返回 Swagger JSON 文档
func swaggerJSON(c *fiber.Ctx) error {
	c.Set("Cache-Control", "public, max-age=3600")
	return c.JSON(docs.SwaggerInfo)
}

// setCacheHeaders 设置静态文件缓存头
func setCacheHeaders(c *fiber.Ctx) error {
	path := c.Path()
	isHTML := filepath.Ext(path) == ".html" || path == "/"

	if !isHTML && c.Response().StatusCode() == fiber.StatusOK {
		c.Response().Header.Set("Cache-Control", oneYearMaxAge)
		return nil
	}

	// HTML文件或错误状态:禁止缓存
	c.Response().Header.Set("Cache-Control", noCache)
	c.Response().Header.Set("Pragma", "no-cache")
	c.Response().Header.Set("Expires", "0")
	return nil
}

// initStorage 初始化Redis连接
func (s *Server) initStorage() error {
	log.Println("正在初始化Redis连接...")
	if err := storage.InitRedis(); err != nil {
		return err
	}
	log.Println("Redis初始化成功")
	return nil
}

// closeRedis 关闭Redis连接
func (s *Server) closeRedis() {
	log.Println("正在关闭Redis连接...")
	if err := storage.CloseRedis(); err != nil {
		log.Printf("关闭Redis连接失败: %v", err)
	} else {
		log.Println("Redis连接已关闭")
	}
}

// printSeparator 打印分隔线
func printSeparator() {
	log.Println(strings.Repeat("=", separatorLength))
}

// printStartupInfo 打印启动信息
func (s *Server) printStartupInfo() {
	printSeparator()
	log.Printf("服务已启动，监听端口: %s", config.DefaultPort)
	log.Printf("存储方式: Redis (%s)", s.cfg.RedisAddr)
	log.Println("数据更新策略: 过期自动更新（容错机制）")
	log.Println("安全措施: 速率限制、安全响应头")
	log.Println("优雅关闭: 已启用（SIGINT/SIGTERM）")
	printSeparator()
}

// preloadData 启动时预加载数据
func (s *Server) preloadData() {
	printSeparator()
	crawler.PreloadData()
	printSeparator()
}

// ===== 以下为兼容性函数（保持向后兼容） =====

// StartCompat 启动服务器的兼容性函数（使用 context.Background）
// 这个函数保持原有签名，方便不关心 context 的调用者
func StartCompat() {
	srv := New()
	ctx := context.Background()

	if err := srv.Start(ctx); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
