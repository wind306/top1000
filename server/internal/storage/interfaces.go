package storage

import (
	"context"
	"encoding/json"
	"top1000/internal/model"
)

// DataStore 数据存储接口（Top1000 数据）
// 小而专注的接口，遵循 Go 接口设计原则
type DataStore interface {
	// LoadData 加载数据（支持 context 取消和超时）
	LoadData(ctx context.Context) (*model.ProcessedData, error)

	// SaveData 保存数据（支持 context 取消和超时）
	SaveData(ctx context.Context, data model.ProcessedData) error

	// DataExists 检查数据是否存在
	DataExists(ctx context.Context) (bool, error)

	// IsDataExpired 检查数据是否过期（基于时间字段）
	IsDataExpired(ctx context.Context) (bool, error)
}

// SitesStore 站点数据存储接口
// 分离关注点，独立接口
type SitesStore interface {
	// LoadSitesData 加载站点数据
	LoadSitesData(ctx context.Context) (json.RawMessage, error)

	// SaveSitesData 保存站点数据
	SaveSitesData(ctx context.Context, data json.RawMessage) error

	// SitesDataExists 检查站点数据是否存在
	SitesDataExists(ctx context.Context) (bool, error)
}

// UpdateLock 更新锁接口（并发控制）
// 分离锁逻辑，方便测试和替换实现
type UpdateLock interface {
	// IsUpdating 检查是否正在更新
	IsUpdating() bool

	// SetUpdating 设置更新标记
	SetUpdating(bool)

	// IsSitesUpdating 检查是否正在更新站点数据
	IsSitesUpdating() bool

	// SetSitesUpdating 设置站点数据更新标记
	SetSitesUpdating(bool)
}
