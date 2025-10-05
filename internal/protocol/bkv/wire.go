package bkv

import (
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
)

// NewHandlers 构造 BKV 处理集合
// P0修复: 删除内存参数存储，直接使用数据库持久化
func NewHandlers(repo *pgstorage.Repository, reason *ReasonMap) *Handlers {
	return &Handlers{Repo: repo, Reason: reason}
}
