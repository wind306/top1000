package crawler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"top1000/internal/config"
	"top1000/internal/model"
)

const validTop1000Payload = `create time 2026-01-19 07:50:56 by xxx

站名：测试站点 【ID：123】
重复度：85.5%
文件大小：1.2TB
`

func TestFetcherFetchWithContext(t *testing.T) {
	tests := []struct {
		name        string
		handler     http.HandlerFunc
		timeout     time.Duration
		wantErr     string
		wantErrIs   error
		wantItemCnt int
	}{
		{
			name: "成功抓取并解析",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(validTop1000Payload))
			},
			timeout:     100 * time.Millisecond,
			wantItemCnt: 1,
		},
		{
			name: "上游返回非200",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "boom", http.StatusBadGateway)
			},
			timeout: 100 * time.Millisecond,
			wantErr: "API返回错误状态码",
		},
		{
			name: "响应内容非法",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("broken payload"))
			},
			timeout: 100 * time.Millisecond,
			wantErr: "数据条目不能为空",
		},
		{
			name: "请求超时会取消",
			handler: func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(50 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(validTop1000Payload))
			},
			timeout:   10 * time.Millisecond,
			wantErrIs: ErrFetchingCancelled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			fetcher := newTestFetcher(server.URL)
			data, err := fetcher.FetchWithContext(ctx)
			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("错误 = %v, want errors.Is(..., %v)", err, tt.wantErrIs)
				}
				return
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("错误 = %v, want contains %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("FetchWithContext() 返回错误: %v", err)
			}
			if len(data.Items) != tt.wantItemCnt {
				t.Fatalf("Items 数量 = %d, want %d", len(data.Items), tt.wantItemCnt)
			}
			if data.Items[0].SiteName != "测试站点" {
				t.Fatalf("SiteName = %s, want 测试站点", data.Items[0].SiteName)
			}
		})
	}
}

func TestFetcherReturnsTaskRunningWhenLockHeld(t *testing.T) {
	lock := &stubLock{locked: true}
	fetcher := newTestFetcher("http://example.com")
	fetcher.lock = lock

	_, err := fetcher.FetchWithContext(context.Background())
	if !errors.Is(err, ErrTaskRunning) {
		t.Fatalf("错误 = %v, want %v", err, ErrTaskRunning)
	}
}

func TestFetcherPreloadDataSavesFetchedData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(validTop1000Payload))
	}))
	defer server.Close()

	store := &stubDataStore{exists: false}
	fetcher := newTestFetcher(server.URL)
	fetcher.PreloadData(context.Background(), store)

	if store.saveCalls != 1 {
		t.Fatalf("SaveData 调用次数 = %d, want 1", store.saveCalls)
	}
	if store.saved == nil || len(store.saved.Items) != 1 {
		t.Fatalf("保存的数据不正确: %+v", store.saved)
	}
}

func TestCheckDataLoadRequired(t *testing.T) {
	tests := []struct {
		name  string
		store *stubDataStore
		want  bool
	}{
		{
			name: "数据不存在时需要加载",
			store: &stubDataStore{
				exists: false,
			},
			want: true,
		},
		{
			name: "数据新鲜时不需要加载",
			store: &stubDataStore{
				exists:  true,
				expired: false,
			},
			want: false,
		},
		{
			name: "数据过期时需要加载",
			store: &stubDataStore{
				exists:  true,
				expired: true,
			},
			want: true,
		},
		{
			name: "检查存在性失败时需要加载",
			store: &stubDataStore{
				existsErr: errors.New("exists failed"),
			},
			want: true,
		},
		{
			name: "检查过期失败时需要加载",
			store: &stubDataStore{
				exists:     true,
				expiredErr: errors.New("expired failed"),
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkDataLoadRequired(context.Background(), tt.store); got != tt.want {
				t.Fatalf("checkDataLoadRequired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFetcherUsesSingleFlightLock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(30 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(validTop1000Payload))
	}))
	defer server.Close()

	lock := &stubLock{}
	fetcher := newTestFetcher(server.URL)
	fetcher.lock = lock

	errCh := make(chan error, 2)
	go func() {
		_, err := fetcher.FetchWithContext(context.Background())
		errCh <- err
	}()
	go func() {
		_, err := fetcher.FetchWithContext(context.Background())
		errCh <- err
	}()

	err1 := <-errCh
	err2 := <-errCh
	if !oneOf(err1, err2, nil, ErrTaskRunning) {
		t.Fatalf("并发结果异常: err1=%v err2=%v", err1, err2)
	}
}

func TestParseResponse(t *testing.T) {
	tests := []struct {
		name      string
		rawData   string
		wantCount int
		wantTime  string
		check     func([]model.SiteItem) error
	}{
		{
			name:      "标准格式",
			rawData:   validTop1000Payload,
			wantCount: 1,
			wantTime:  "2026-01-19 07:50:56",
			check: func(items []model.SiteItem) error {
				if items[0].SiteName != "测试站点" {
					t.Errorf("SiteName = %v, want %v", items[0].SiteName, "测试站点")
				}
				if items[0].SiteID != "123" {
					t.Errorf("SiteID = %v, want %v", items[0].SiteID, "123")
				}
				if items[0].Duplication != "85.5%" {
					t.Errorf("Duplication = %v, want %v", items[0].Duplication, "85.5%")
				}
				if items[0].Size != "1.2TB" {
					t.Errorf("Size = %v, want %v", items[0].Size, "1.2TB")
				}
				return nil
			},
		},
		{
			name: "多条数据",
			rawData: `create time 2026-01-19 07:50:56 by xxx

站名：站点1 【ID：1】
重复度：80%
文件大小：1TB
站名：站点2 【ID：2】
重复度：90%
文件大小：2TB
站名：站点3 【ID：3】
重复度：95%
文件大小：3TB
`,
			wantCount: 3,
			wantTime:  "2026-01-19 07:50:56",
		},
		{
			name: "Windows换行符",
			rawData: "create time 2026-01-19 07:50:56 by xxx\r\n" +
				"\r\n" +
				"站名：测试站点 【ID：123】\r\n" +
				"重复度：85.5%\r\n" +
				"文件大小：1.2TB\r\n",
			wantCount: 1,
			wantTime:  "2026-01-19 07:50:56",
		},
		{
			name: "数据行不完整(跳过)",
			rawData: `create time 2026-01-19 07:50:56 by xxx

站名：站点1 【ID：1】
重复度：80%
文件大小：1TB
站名：站点2 【ID：2】
重复度：90%
站名：站点3 【ID：3】
文件大小：3TB
`,
			wantCount: 2,
			wantTime:  "2026-01-19 07:50:56",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processed := parseResponse(tt.rawData)
			if processed.Time != tt.wantTime {
				t.Errorf("parseResponse() Time = %v, want %v", processed.Time, tt.wantTime)
			}
			if len(processed.Items) != tt.wantCount {
				t.Errorf("parseResponse() Items length = %v, want %v", len(processed.Items), tt.wantCount)
			}
			if tt.check != nil && len(processed.Items) > 0 {
				if err := tt.check(processed.Items); err != nil {
					t.Errorf("parseResponse() check failed: %v", err)
				}
			}
		})
	}
}

func TestExtractFieldValue(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "标准格式",
			line: "重复度：85.5%",
			want: "85.5%",
		},
		{
			name: "带空格",
			line: "文件大小： 1.2TB",
			want: "1.2TB",
		},
		{
			name: "无冒号",
			line: "纯文本",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractFieldValue(tt.line); got != tt.want {
				t.Errorf("extractFieldValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractTime(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "标准格式",
			raw:  "create time 2026-01-19 07:50:56 by xxx",
			want: "2026-01-19 07:50:56",
		},
		{
			name: "无后缀",
			raw:  "create time 2026-01-19 07:50:56",
			want: "2026-01-19 07:50:56",
		},
		{
			name: "空字符串",
			raw:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractTime(tt.raw); got != tt.want {
				t.Errorf("extractTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeLineEndings(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "Windows换行符",
			input: "line1\r\nline2\r\n",
			want:  "line1\nline2\n",
		},
		{
			name:  "Unix换行符",
			input: "line1\nline2\n",
			want:  "line1\nline2\n",
		},
		{
			name:  "混合换行符",
			input: "line1\r\nline2\nline3\r\n",
			want:  "line1\nline2\nline3\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeLineEndings(tt.input); got != tt.want {
				t.Errorf("normalizeLineEndings() = %v, want %v", got, tt.want)
			}
		})
	}
}

func newTestFetcher(apiURL string) *Fetcher {
	return &Fetcher{
		apiURL:        apiURL,
		timeout:       100 * time.Millisecond,
		maxRetries:    1,
		retryInterval: time.Millisecond,
		lock:          &stubLock{},
		getConfig:     func() *config.Config { return &config.Config{} },
		newClient:     httpclientFactory,
	}
}

func httpclientFactory(ctx context.Context, timeout time.Duration, insecureSkipVerify bool) *http.Client {
	return &http.Client{Timeout: timeout}
}

type stubLock struct {
	mu     sync.Mutex
	locked bool
}

func (s *stubLock) TryLock() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locked {
		return false
	}
	s.locked = true
	return true
}

func (s *stubLock) Unlock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.locked = false
}

type stubDataStore struct {
	saved      *model.ProcessedData
	saveErr    error
	saveCalls  int
	exists     bool
	existsErr  error
	expired    bool
	expiredErr error
}

func (s *stubDataStore) LoadData(context.Context) (*model.ProcessedData, error) {
	return s.saved, nil
}

func (s *stubDataStore) SaveData(_ context.Context, data model.ProcessedData) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saveCalls++
	dataCopy := data
	s.saved = &dataCopy
	s.exists = true
	return nil
}

func (s *stubDataStore) DataExists(context.Context) (bool, error) {
	if s.existsErr != nil {
		return false, s.existsErr
	}
	return s.exists, nil
}

func (s *stubDataStore) IsDataExpired(context.Context) (bool, error) {
	if s.expiredErr != nil {
		return false, s.expiredErr
	}
	return s.expired, nil
}

func oneOf(err1, err2 error, want1, want2 error) bool {
	matches := func(err, want error) bool {
		if want == nil {
			return err == nil
		}
		return errors.Is(err, want)
	}
	return (matches(err1, want1) && matches(err2, want2)) || (matches(err1, want2) && matches(err2, want1))
}
