package model

import (
	"regexp"
)

// 验证错误常量 - 遵循 DRY 原则
const (
	errSiteNameEmpty   = "站点名称不能为空"
	errSiteIDEmpty     = "站点ID不能为空"
	errSiteIDInvalid   = "站点ID必须是数字"
	errDuplicationInvalid = "重复度必须为数字"
	errSizeInvalid     = "文件大小格式错误"
	errIDInvalid       = "ID必须大于0"
	errTimeEmpty       = "时间不能为空"
	errItemsEmpty      = "数据条目不能为空"
	errItemValidateFailed = "第%d条数据验证失败"
)

var (
	sizePattern = regexp.MustCompile(`^\d+(\.\d+)?\s*(KB|MB|GB|TB)$`)
)
