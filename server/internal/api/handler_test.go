package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"top1000/internal/config"
	"top1000/internal/model"
	"top1000/internal/storage"
)

func TestRefreshDataSkipsWhenUpdateInProgress(t *testing.T) {
	store := &stubDataStore{}
	lock := &stubUpdateLock{updating: true}
	crawler := &stubCrawler{}
	handler := newTestHandler(store, &stubSitesStore{}, lock, crawler, &stubSitesFetcher{}, &config.Config{})

	if err := handler.refreshData(context.Background()); err != nil {
		t.Fatalf("refreshData() 返回错误: %v", err)
	}

	if crawler.calls != 0 {
		t.Fatalf("crawler 调用次数 = %d, want 0", crawler.calls)
	}
}

func TestRefreshDataSavesFetchedData(t *testing.T) {
	freshData := model.ProcessedData{
		Time: "2026-01-19 07:50:56",
		Items: []model.SiteItem{
			{ID: 1, SiteName: "新站点", SiteID: "101"},
		},
	}
	store := &stubDataStore{
		data:       &model.ProcessedData{Time: "2026-01-18 07:50:56", Items: []model.SiteItem{{ID: 1, SiteName: "旧站点", SiteID: "100"}}},
		dataExists: true,
		expired:    true,
	}
	lock := &stubUpdateLock{}
	crawler := &stubCrawler{data: &freshData}
	handler := newTestHandler(store, &stubSitesStore{}, lock, crawler, &stubSitesFetcher{}, &config.Config{})

	if err := handler.refreshData(context.Background()); err != nil {
		t.Fatalf("refreshData() 返回错误: %v", err)
	}

	if store.saveCalls != 1 {
		t.Fatalf("SaveData 调用次数 = %d, want 1", store.saveCalls)
	}
	if store.data == nil || store.data.Time != freshData.Time {
		t.Fatalf("保存的数据不正确: %+v", store.data)
	}
	if lock.updating {
		t.Fatal("refreshData() 结束后 updating 仍为 true")
	}
}

func TestGetTop1000DataReturnsCachedDataWhenRefreshFails(t *testing.T) {
	oldData := model.ProcessedData{
		Time: "2026-01-19 07:50:56",
		Items: []model.SiteItem{
			{ID: 1, SiteName: "缓存站点", SiteID: "123"},
		},
	}
	handler := newTestHandler(
		&stubDataStore{
			data:       &oldData,
			dataExists: true,
			expired:    true,
		},
		&stubSitesStore{},
		&stubUpdateLock{},
		&stubCrawler{err: errors.New("crawler failed")},
		&stubSitesFetcher{},
		&config.Config{},
	)

	resp := performRequest(t, handler, "/top1000.json")
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("状态码 = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}

	var body model.ProcessedData
	decodeResponse(t, resp, &body)
	if body.Time != oldData.Time {
		t.Fatalf("Time = %s, want %s", body.Time, oldData.Time)
	}
	if len(body.Items) != 1 || body.Items[0].SiteName != "缓存站点" {
		t.Fatalf("返回数据不正确: %+v", body.Items)
	}
}

func TestGetTop1000DataReturns500WhenLoadFails(t *testing.T) {
	handler := newTestHandler(
		&stubDataStore{
			loadErr: errors.New("load failed"),
		},
		&stubSitesStore{},
		&stubUpdateLock{},
		&stubCrawler{err: errors.New("crawler failed")},
		&stubSitesFetcher{},
		&config.Config{},
	)

	resp := performRequest(t, handler, "/top1000.json")
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("状态码 = %d, want %d", resp.StatusCode, fiber.StatusInternalServerError)
	}
}

func TestRefreshSitesDataSkipsWhenUpdateInProgress(t *testing.T) {
	sitesFetcher := &stubSitesFetcher{}
	lock := &stubUpdateLock{sitesUpdating: true}
	handler := newTestHandler(&stubDataStore{}, &stubSitesStore{}, lock, &stubCrawler{}, sitesFetcher, &config.Config{IYYUSign: "sign"})

	if err := handler.refreshSitesData(context.Background(), "sign"); err != nil {
		t.Fatalf("refreshSitesData() 返回错误: %v", err)
	}

	if sitesFetcher.calls != 0 {
		t.Fatalf("FetchSitesData 调用次数 = %d, want 0", sitesFetcher.calls)
	}
}

func TestGetSitesDataRequiresSign(t *testing.T) {
	handler := newTestHandler(&stubDataStore{}, &stubSitesStore{}, &stubUpdateLock{}, &stubCrawler{}, &stubSitesFetcher{}, &config.Config{})

	resp := performRequest(t, handler, "/sites.json")
	if resp.StatusCode != fiber.StatusBadGateway {
		t.Fatalf("状态码 = %d, want %d", resp.StatusCode, fiber.StatusBadGateway)
	}
}

func TestGetSitesDataRefreshesAndReturnsStoredData(t *testing.T) {
	sitesStore := &stubSitesStore{}
	sitesFetcher := &stubSitesFetcher{
		data: json.RawMessage(`{"site1":{"name":"站点1"}}`),
	}
	handler := newTestHandler(
		&stubDataStore{},
		sitesStore,
		&stubUpdateLock{},
		&stubCrawler{},
		sitesFetcher,
		&config.Config{IYYUSign: "test-sign"},
	)

	resp := performRequest(t, handler, "/sites.json")
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("状态码 = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
	if got := resp.Header.Get("Cache-Control"); got != "public, max-age=3600" {
		t.Fatalf("Cache-Control = %s, want public, max-age=3600", got)
	}
	if sitesFetcher.calls != 1 {
		t.Fatalf("FetchSitesData 调用次数 = %d, want 1", sitesFetcher.calls)
	}
	if sitesStore.saveCalls != 1 {
		t.Fatalf("SaveSitesData 调用次数 = %d, want 1", sitesStore.saveCalls)
	}

	var body map[string]map[string]string
	decodeResponse(t, resp, &body)
	if body["site1"]["name"] != "站点1" {
		t.Fatalf("返回数据不正确: %+v", body)
	}
}

func TestGetSitesDataReturnsCachedDataWhenRefreshFails(t *testing.T) {
	sitesStore := &stubSitesStore{
		data:      json.RawMessage(`{"site1":{"name":"缓存站点"}}`),
		existsErr: errors.New("exists failed"),
	}
	handler := newTestHandler(
		&stubDataStore{},
		sitesStore,
		&stubUpdateLock{},
		&stubCrawler{},
		&stubSitesFetcher{err: errors.New("fetch failed")},
		&config.Config{IYYUSign: "test-sign"},
	)

	resp := performRequest(t, handler, "/sites.json")
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("状态码 = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}

	var body map[string]map[string]string
	decodeResponse(t, resp, &body)
	if body["site1"]["name"] != "缓存站点" {
		t.Fatalf("返回数据不正确: %+v", body)
	}
}

func newTestHandler(
	store storage.DataStore,
	sitesStore storage.SitesStore,
	lock *stubUpdateLock,
	crawler *stubCrawler,
	sitesFetcher *stubSitesFetcher,
	cfg *config.Config,
) *Handler {
	return newHandler(store, sitesStore, lock, crawler, sitesFetcher, func() *config.Config {
		return cfg
	})
}

type stubDataStore struct {
	data       *model.ProcessedData
	loadErr    error
	saveErr    error
	dataExists bool
	existsErr  error
	expired    bool
	expiredErr error
	saveCalls  int
}

func (s *stubDataStore) LoadData(context.Context) (*model.ProcessedData, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	if s.data == nil {
		return nil, errors.New("no data")
	}
	return s.data, nil
}

func (s *stubDataStore) SaveData(_ context.Context, data model.ProcessedData) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saveCalls++
	dataCopy := data
	s.data = &dataCopy
	s.dataExists = true
	return nil
}

func (s *stubDataStore) DataExists(context.Context) (bool, error) {
	if s.existsErr != nil {
		return false, s.existsErr
	}
	return s.dataExists, nil
}

func (s *stubDataStore) IsDataExpired(context.Context) (bool, error) {
	if s.expiredErr != nil {
		return false, s.expiredErr
	}
	return s.expired, nil
}

type stubSitesStore struct {
	data      json.RawMessage
	loadErr   error
	saveErr   error
	exists    bool
	existsErr error
	saveCalls int
}

func (s *stubSitesStore) LoadSitesData(context.Context) (json.RawMessage, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	if s.data == nil {
		return nil, errors.New("no sites data")
	}
	return s.data, nil
}

func (s *stubSitesStore) SaveSitesData(_ context.Context, data json.RawMessage) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saveCalls++
	s.data = append(json.RawMessage(nil), data...)
	s.exists = true
	return nil
}

func (s *stubSitesStore) SitesDataExists(context.Context) (bool, error) {
	if s.existsErr != nil {
		return false, s.existsErr
	}
	return s.exists, nil
}

type stubUpdateLock struct {
	updating      bool
	sitesUpdating bool
}

func (s *stubUpdateLock) IsUpdating() bool {
	return s.updating
}

func (s *stubUpdateLock) SetUpdating(updating bool) {
	s.updating = updating
}

func (s *stubUpdateLock) IsSitesUpdating() bool {
	return s.sitesUpdating
}

func (s *stubUpdateLock) SetSitesUpdating(updating bool) {
	s.sitesUpdating = updating
}

type stubCrawler struct {
	data  *model.ProcessedData
	err   error
	calls int
}

func (s *stubCrawler) FetchWithContext(context.Context) (*model.ProcessedData, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.data, nil
}

type stubSitesFetcher struct {
	data  json.RawMessage
	err   error
	calls int
}

func (s *stubSitesFetcher) FetchSitesData(context.Context, string) (json.RawMessage, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.data, nil
}

func performRequest(t *testing.T, handler *Handler, path string) *http.Response {
	t.Helper()

	app := fiber.New()
	handler.RegisterRoutes(app)

	req := httptest.NewRequest("GET", path, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() 失败: %v", err)
	}
	return resp
}

func decodeResponse(t *testing.T, resp *http.Response, target any) {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应失败: %v", err)
	}

	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
}
