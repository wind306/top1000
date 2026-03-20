package api

import (
	"context"
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
	"top1000/internal/httpclient"
	"top1000/internal/model"
	"top1000/internal/storage"
)

const (
	dataUpdateLogPrefix  = "Top1000"
	sitesUpdateLogPrefix = "Sites"
	defaultAPITimeout    = 15 * time.Second
)

// Handler API 处理器（依赖注入模式）
type Handler struct {
	store      storage.DataStore
	sitesStore storage.SitesStore
	lock       storage.UpdateLock
	crawler    Crawler
	sites      SitesFetcher
	getConfig  func() *config.Config
}

// Crawler 爬虫接口（小而专注）
// 定义爬虫的核心能力，方便测试和替换实现
type Crawler interface {
	// FetchWithContext 带 context 的数据爬取
	FetchWithContext(ctx context.Context) (*model.ProcessedData, error)
}

// SitesFetcher 站点数据抓取接口
type SitesFetcher interface {
	FetchSitesData(ctx context.Context, sign string) (json.RawMessage, error)
}

// NewHandler 创建 Handler 实例（依赖注入）
func NewHandler(store storage.DataStore, sitesStore storage.SitesStore, lock storage.UpdateLock) *Handler {
	return newHandler(store, sitesStore, lock, crawler.NewFetcher(), &defaultSitesFetcher{getConfig: config.Get}, config.Get)
}

func newHandler(
	store storage.DataStore,
	sitesStore storage.SitesStore,
	lock storage.UpdateLock,
	crawler Crawler,
	sites SitesFetcher,
	getConfig func() *config.Config,
) *Handler {
	return &Handler{
		store:      store,
		sitesStore: sitesStore,
		lock:       lock,
		crawler:    crawler,
		sites:      sites,
		getConfig:  getConfig,
	}
}

type defaultSitesFetcher struct {
	getConfig func() *config.Config
}

func (f *defaultSitesFetcher) FetchSitesData(ctx context.Context, sign string) (json.RawMessage, error) {
	apiURL, err := buildSitesAPIURL(sign)
	if err != nil {
		return nil, fmt.Errorf("解析基础URL失败: %w", err)
	}

	client := httpclient.New(ctx, 5*time.Second, f.getConfig().InsecureSkipVerify)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("站点API返回错误状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if !json.Valid(body) {
		return nil, fmt.Errorf("解析JSON失败: invalid JSON payload")
	}

	return json.RawMessage(body), nil
}

func buildSitesAPIURL(sign string) (string, error) {
	apiURL, err := url.Parse("https://api.iyuu.cn/index.php")
	if err != nil {
		return "", err
	}

	params := url.Values{}
	params.Add("service", "App.Api.Sites")
	params.Add("sign", sign)
	params.Add("version", "2.0.0")
	apiURL.RawQuery = params.Encode()

	return apiURL.String(), nil
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(app *fiber.App) {
	app.Get("/top1000.json", h.GetTop1000Data)
	app.Get("/sites.json", h.GetSitesData)
}

// ===== 以下改为 Handler 的方法 =====

// GetTop1000Data 提供 Top1000 数据接口。
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
	return withUpdateGuard(dataUpdateLogPrefix, h.lock.IsUpdating, h.lock.SetUpdating, func() error {
		// 保存旧数据用于容错（传递context）
		oldData, err := h.store.LoadData(ctx)
		if err != nil {
			log.Printf("[%s] 加载旧数据失败: %v", dataUpdateLogPrefix, err)
			// 容错：旧数据不存在时继续爬取新数据
		}

		log.Printf("[%s] 开始爬取新数据...", dataUpdateLogPrefix)
		newData, err := h.crawler.FetchWithContext(ctx)
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
	})
}

// GetSitesData 提供 IYUU 站点数据接口。
func (h *Handler) GetSitesData(c *fiber.Ctx) error {
	cfg := h.getConfig()

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
func (h *Handler) refreshSitesData(ctx context.Context, sign string) error {
	return withUpdateGuard(sitesUpdateLogPrefix, h.lock.IsSitesUpdating, h.lock.SetSitesUpdating, func() error {
		log.Printf("[%s] 开始获取站点数据...", sitesUpdateLogPrefix)

		result, err := h.sites.FetchSitesData(ctx, sign)
		if err != nil {
			log.Printf("[%s] 获取站点数据失败: %v", sitesUpdateLogPrefix, err)
			return err
		}

		// 保存到存储（24小时TTL）
		if err := h.sitesStore.SaveSitesData(ctx, result); err != nil {
			log.Printf("[%s] 保存数据失败: %v", sitesUpdateLogPrefix, err)
			return fmt.Errorf("保存数据失败: %w", err)
		}

		log.Printf("[%s] 站点数据更新成功", sitesUpdateLogPrefix)
		return nil
	})
}

func withUpdateGuard(logPrefix string, isUpdating func() bool, setUpdating func(bool), fn func() error) error {
	if isUpdating() {
		log.Printf("[%s] 正在更新中，跳过", logPrefix)
		return nil
	}

	setUpdating(true)
	defer setUpdating(false)

	return fn()
}
