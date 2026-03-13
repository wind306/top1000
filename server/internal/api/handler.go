package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/gofiber/fiber/v2"
	"top1000/internal/config"
	"top1000/internal/crawler"
	"top1000/internal/model"
	"top1000/internal/storage"
)

const (
	dataUpdateLogPrefix  = "Top1000"
	sitesUpdateLogPrefix = "Sites"
	defaultAPITimeout    = 15 * time.Second
)

// Go 1.26: 创建HTTP客户端的辅助函数，避免重复代码
// 这个SB函数根据context和配置创建合适的HTTP客户端
func createHTTPClient(ctx context.Context, timeout time.Duration) *http.Client {
	cfg := config.Get()

	// 根据context的deadline调整超时
	actualTimeout := timeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			actualTimeout = remaining
		}
	}

	client := &http.Client{Timeout: actualTimeout}

	if cfg.InsecureSkipVerify {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	return client
}

// Handler API 处理器（依赖注入模式）
type Handler struct {
	store      storage.DataStore
	sitesStore storage.SitesStore
	lock       storage.UpdateLock
	crawler    Crawler
}

// Crawler 爬虫接口（小而专注）
// 定义爬虫的核心能力，方便测试和替换实现
type Crawler interface {
	// FetchTop1000WithContext 带 context 的数据爬取
	FetchTop1000WithContext(ctx context.Context) (*model.ProcessedData, error)
}

// NewHandler 创建 Handler 实例（依赖注入）
func NewHandler(store storage.DataStore, sitesStore storage.SitesStore, lock storage.UpdateLock) *Handler {
	return &Handler{
		store:      store,
		sitesStore: sitesStore,
		lock:       lock,
		crawler:    &defaultCrawler{},
	}
}

// defaultCrawler 默认爬虫实现（实现 Crawler 接口）
type defaultCrawler struct{}

// FetchTop1000WithContext 调用底层爬虫
func (d *defaultCrawler) FetchTop1000WithContext(ctx context.Context) (*model.ProcessedData, error) {
	return crawler.FetchTop1000WithContext(ctx)
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(app *fiber.App) {
	app.Get("/top1000.json", h.GetTop1000Data)
	app.Get("/sites.json", h.GetSitesData)
}

// ===== 以下改为 Handler 的方法 =====

// GetTop1000Data 提供Top1000数据的API接口
// @Summary 获取Top1000站点数据
// @Description 获取Top1000站点列表数据，数据会自动更新（24小时过期）
// @Tags Top1000
// @Accept json
// @Produce json
// @Success 200 {object} model.ProcessedData
// @Failure 500 {object} map[string]string "error": "无法加载数据"
// @Router /top1000.json [get]
func (h *Handler) GetTop1000Data(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), defaultAPITimeout)
	defer cancel()

	if h.shouldUpdateData(ctx) {
		if err := h.refreshData(ctx); err != nil {
			log.Printf("[%s] 刷新数据失败: %v", dataUpdateLogPrefix, err)
		}
	}

	data, err := h.store.LoadData(ctx)
	if err != nil {
		log.Printf("[%s] 加载数据失败: %v", dataUpdateLogPrefix, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "无法加载数据",
		})
	}

	return c.JSON(data)
}

// shouldUpdateData 检查数据是否需要更新
func (h *Handler) shouldUpdateData(ctx context.Context) bool {
	exists, err := h.store.DataExists(ctx)
	if err != nil || !exists {
		return true
	}

	isExpired, err := h.store.IsDataExpired(ctx)
	return err != nil || isExpired
}

// refreshData 刷新数据（带容错机制）
// 返回 error 让调用者知道刷新是否成功
func (h *Handler) refreshData(ctx context.Context) error {
	// 防止并发更新
	if h.lock.IsUpdating() {
		log.Printf("[%s] 正在更新中，跳过", dataUpdateLogPrefix)
		return nil
	}

	h.lock.SetUpdating(true)
	defer h.lock.SetUpdating(false)

	// 保存旧数据用于容错（传递context）
	oldData, err := h.store.LoadData(ctx)
	if err != nil {
		log.Printf("[%s] 加载旧数据失败: %v", dataUpdateLogPrefix, err)
		// 容错：旧数据不存在时继续爬取新数据
	}

	log.Printf("[%s] 开始爬取新数据...", dataUpdateLogPrefix)
	newData, err := h.crawler.FetchTop1000WithContext(ctx)
	if err != nil {
		// 爬取失败，如果有旧数据则使用旧数据（容错）
		if oldData != nil {
			log.Printf("[%s] 爬取失败，使用旧数据: %v", dataUpdateLogPrefix, err)
			return err
		}
		log.Printf("[%s] 爬取失败且无旧数据: %v", dataUpdateLogPrefix, err)
		return err
	}

	if err := h.store.SaveData(ctx, *newData); err != nil {
		log.Printf("[%s] 保存数据失败: %v", dataUpdateLogPrefix, err)
		return err
	}

	log.Printf("[%s] 数据更新成功（%d 条）", dataUpdateLogPrefix, len(newData.Items))
	return nil
}

// GetSitesData 提供IYUU站点数据的API接口
// @Summary 获取IYUU站点列表
// @Description 获取IYUU站点列表数据（需要配置IYUU_SIGN环境变量）
// @Tags Sites
// @Accept json
// @Produce json
// @Success 200 {object} interface{} "站点列表数据"
// @Failure 502 {object} map[string]string "error": "未配置IYUU_SIGN环境变量"
// @Failure 500 {object} map[string]string "error": "无法加载站点数据"
// @Router /sites.json [get]
func (h *Handler) GetSitesData(c *fiber.Ctx) error {
	cfg := config.Get()

	if cfg.IYYUSign == "" {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "未配置IYUU_SIGN环境变量",
		})
	}

	ctx, cancel := context.WithTimeout(c.Context(), defaultAPITimeout)
	defer cancel()

	if h.shouldUpdateSitesData(ctx) {
		if err := h.refreshSitesData(ctx, cfg.IYYUSign); err != nil {
			log.Printf("[%s] 刷新站点数据失败: %v", sitesUpdateLogPrefix, err)
		}
	}

	data, err := h.sitesStore.LoadSitesData(ctx)
	if err != nil {
		log.Printf("[%s] 加载站点数据失败: %v", sitesUpdateLogPrefix, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "无法加载站点数据",
		})
	}

	c.Set("Content-Type", "application/json; charset=utf-8")
	c.Set("Cache-Control", "public, max-age=3600")

	return c.JSON(data)
}

// shouldUpdateSitesData 检查站点数据是否需要更新
func (h *Handler) shouldUpdateSitesData(ctx context.Context) bool {
	exists, err := h.sitesStore.SitesDataExists(ctx)
	return err != nil || !exists
}

// refreshSitesData 刷新站点数据（带容错机制）
// 返回 error 让调用者知道刷新是否成功
// Go 1.26: 使用 createHTTPClient 辅助函数，DRY原则落地
func (h *Handler) refreshSitesData(ctx context.Context, sign string) error {
	// 防止并发更新
	if h.lock.IsSitesUpdating() {
		log.Printf("[%s] 正在更新中，跳过", sitesUpdateLogPrefix)
		return nil
	}

	h.lock.SetSitesUpdating(true)
	defer h.lock.SetSitesUpdating(false)

	log.Printf("[%s] 开始获取站点数据...", sitesUpdateLogPrefix)

	apiURL, err := url.Parse("https://api.iyuu.cn/index.php")
	if err != nil {
		log.Printf("[%s] 解析基础URL失败: %v", sitesUpdateLogPrefix, err)
		return fmt.Errorf("解析基础URL失败: %w", err)
	}
	params := url.Values{}
	params.Add("service", "App.Api.Sites")
	params.Add("sign", sign)
	params.Add("version", "2.0.0")
	apiURL.RawQuery = params.Encode()

	// Go 1.26: 使用辅助函数创建HTTP客户端
	client := createHTTPClient(ctx, 5*time.Second)

	resp, err := client.Get(apiURL.String())
	if err != nil {
		log.Printf("[%s] 请求失败: %v", sitesUpdateLogPrefix, err)
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// Go 1.26: io.ReadAll 性能已优化，分配更少内存
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[%s] 读取响应失败: %v", sitesUpdateLogPrefix, err)
		return fmt.Errorf("读取响应失败: %w", err)
	}

	// 解析JSON
	var result any
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("[%s] 解析JSON失败: %v", sitesUpdateLogPrefix, err)
		return fmt.Errorf("解析JSON失败: %w", err)
	}

	// 保存到存储（24小时TTL）
	if err := h.sitesStore.SaveSitesData(ctx, result); err != nil {
		log.Printf("[%s] 保存数据失败: %v", sitesUpdateLogPrefix, err)
		return fmt.Errorf("保存数据失败: %w", err)
	}

	log.Printf("[%s] 站点数据更新成功", sitesUpdateLogPrefix)
	return nil
}

