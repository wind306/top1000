package storage

// 错误常量 - 遵循 DRY 原则，避免重复的字符串
const (
	errDataNotFound      = "数据不存在"
	errSitesNotFound     = "站点数据不存在"
	errRedisReadFailed   = "从Redis读取数据失败"
	errRedisSaveFailed   = "保存数据到Redis失败"
	errJSONMarshalFailed = "序列化数据失败"
	errJSONUnmarshalFailed = "解析JSON失败"
	errDataInvalid       = "数据验证失败"
	errCheckExistsFailed = "检查数据存在性失败"
)
