package crawler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"top1000/internal/config"
	"top1000/internal/httpclient"
	"top1000/internal/model"
	"top1000/internal/storage"
)

const (
	logPrefix      = "爬虫"
	httpTimeout    = 10 * time.Second
	maxRetries     = 1
	retryInterval  = 1 * time.Second
	linesPerItem   = 3
	timeLineIndex  = 0
	dataStartLine  = 2
	timePrefix     = "create time "
	timeSuffix     = " by "
	fieldSeparator = "："
	sitePattern    = `站名：(.*?) 【ID：(\d+)】`
)

var (
	siteRegex = regexp.MustCompile(sitePattern)
	taskMutex sync.Mutex
	// Go 1.26: 哨兵错误，用于 errors.Is 检查
	ErrFetchingCancelled = errors.New("爬取被取消")
	ErrTaskRunning       = errors.New("任务正在执行中")
)

type tryLocker interface {
	TryLock() bool
	Unlock()
}

// Fetcher 封装 Top1000 抓取依赖，方便测试替换。
type Fetcher struct {
	apiURL        string
	timeout       time.Duration
	maxRetries    int
	retryInterval time.Duration
	lock          tryLocker
	getConfig     func() *config.Config
	newClient     func(context.Context, time.Duration, bool) *http.Client
}

// NewFetcher 创建默认抓取器。
func NewFetcher() *Fetcher {
	return &Fetcher{
		apiURL:        config.DefaultAPIURL,
		timeout:       httpTimeout,
		maxRetries:    maxRetries,
		retryInterval: retryInterval,
		lock:          &taskMutex,
		getConfig:     config.Get,
		newClient:     httpclient.New,
	}
}

// FetchWithContext 使用配置好的抓取器获取数据。
func (f *Fetcher) FetchWithContext(ctx context.Context) (*model.ProcessedData, error) {
	if !f.lock.TryLock() {
		return nil, ErrTaskRunning
	}
	defer f.lock.Unlock()

	var lastErr error
	for attempt := 0; attempt < f.maxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: %v", ErrFetchingCancelled, ctx.Err())
		}

		if attempt > 0 {
			log.Printf("[%s] 第 %d 次重试...", logPrefix, attempt)

			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("%w: %v", ErrFetchingCancelled, ctx.Err())
			case <-time.After(f.retryInterval):
			}
		}

		data, err := f.doFetchWithContext(ctx)
		if err == nil {
			return data, nil
		}
		lastErr = err
		log.Printf("[%s] 第 %d 次尝试失败: %v", logPrefix, attempt+1, err)
	}

	return nil, lastErr
}

// doFetchWithContext 执行HTTP请求获取数据
func (f *Fetcher) doFetchWithContext(ctx context.Context) (*model.ProcessedData, error) {
	log.Printf("[%s] 开始爬取IYUU数据...", logPrefix)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	client := f.newClient(ctx, f.timeout, f.getConfig().InsecureSkipVerify)
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: %v", ErrFetchingCancelled, ctx.Err())
		}
		return nil, fmt.Errorf("获取数据失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API返回错误状态码: %d", resp.StatusCode)
	}

	// Go 1.26: io.ReadAll 性能已优化（约2倍速度，一半内存分配）
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}

	log.Printf("[%s] 数据获取成功（%d 字节）", logPrefix, len(body))

	processed := parseResponse(string(body))
	if err := processed.Validate(); err != nil {
		log.Printf("[%s] 数据验证失败: %v", logPrefix, err)
		return nil, err
	}

	return &processed, nil
}

// parseResponse 解析原始文本为结构化数据
func parseResponse(rawData string) model.ProcessedData {
	lines := strings.Split(normalizeLineEndings(rawData), "\n")

	var timeLine string
	if len(lines) > 0 {
		timeLine = lines[timeLineIndex]
	}

	var dataLines []string
	if len(lines) > dataStartLine {
		dataLines = lines[dataStartLine:]
	}

	items, skippedCount := parseDataLines(dataLines)

	logParsingWarnings(dataLines, skippedCount)
	log.Printf("[%s] 数据解析完成（%d 条）", logPrefix, len(items))

	return model.ProcessedData{
		Time:  extractTime(timeLine),
		Items: items,
	}
}

// normalizeLineEndings 统一换行符为\n
func normalizeLineEndings(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// parseDataLines 解析数据行
func parseDataLines(dataLines []string) ([]model.SiteItem, int) {
	var items []model.SiteItem
	skippedCount := 0

	for i := 0; i <= len(dataLines)-linesPerItem; i += linesPerItem {
		group := dataLines[i : i+linesPerItem]

		item, ok := parseItemGroup(group)
		if !ok {
			skippedCount++
			continue
		}

		item.ID = len(items) + 1
		items = append(items, item)
	}

	return items, skippedCount
}

// parseItemGroup 解析单组数据（3行）
func parseItemGroup(group []string) (model.SiteItem, bool) {
	match := siteRegex.FindStringSubmatch(group[0])
	if len(match) < 3 {
		return model.SiteItem{}, false
	}

	return model.SiteItem{
		SiteName:    match[1],
		SiteID:      match[2],
		Duplication: extractFieldValue(group[1]),
		Size:        extractFieldValue(group[2]),
	}, true
}

// extractFieldValue 从"字段名：值"格式中提取值
func extractFieldValue(line string) string {
	_, value, ok := strings.Cut(line, fieldSeparator)
	if ok {
		return strings.TrimSpace(value)
	}
	return ""
}

// logParsingWarnings 记录解析警告
func logParsingWarnings(dataLines []string, skippedCount int) {
	remainingLines := len(dataLines) % linesPerItem
	if remainingLines != 0 {
		log.Printf("[%s] 警告：剩余 %d 行未处理", logPrefix, remainingLines)
	}
	if skippedCount > 0 {
		log.Printf("[%s] 警告：跳过 %d 条格式错误的数据", logPrefix, skippedCount)
	}
}

// extractTime 提取时间字符串，去除前缀和后缀
func extractTime(rawTime string) string {
	rawTime = strings.TrimPrefix(rawTime, timePrefix)
	if value, _, ok := strings.Cut(rawTime, timeSuffix); ok {
		return value
	}
	return rawTime
}

// PreloadData 启动时预加载数据
func PreloadData(store storage.DataStore) {
	log.Println("[爬虫] 检查是否需要预加载数据...")

	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()

	if !checkDataLoadRequired(ctx, store) {
		log.Println("[爬虫] Redis中已有新鲜数据，无需预加载")
		return
	}

	log.Println("[爬虫] Redis中无数据或数据过期，开始预加载...")
	NewFetcher().PreloadData(ctx, store)
}

// checkDataLoadRequired 检查是否需要加载数据
func checkDataLoadRequired(ctx context.Context, store storage.DataStore) bool {
	exists, err := store.DataExists(ctx)
	if err != nil || !exists {
		if err != nil {
			log.Printf("[爬虫] 检查数据存在性失败: %v", err)
		}
		return true
	}

	isExpired, err := store.IsDataExpired(ctx)
	if err != nil {
		log.Printf("[爬虫] 检查数据过期失败: %v", err)
		return true
	}

	return isExpired
}

// PreloadData 预加载数据到指定 store。
func (f *Fetcher) PreloadData(ctx context.Context, store storage.DataStore) {
	data, err := f.FetchWithContext(ctx)
	if err != nil {
		log.Printf("[爬虫] 预加载失败: %v", err)
		log.Printf("[爬虫] 提示：首次访问时会自动重试获取数据")
		return
	}

	if err := store.SaveData(ctx, *data); err != nil {
		log.Printf("[爬虫] 保存预加载数据失败: %v", err)
		return
	}

	log.Printf("[爬虫] 预加载成功，已存入Redis（共 %d 条记录）", len(data.Items))
}
