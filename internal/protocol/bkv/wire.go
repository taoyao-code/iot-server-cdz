package bkv

import (
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
)

// NewHandlers 构造 BKV 处理集合
func NewHandlers(repo *pgstorage.Repository, reason *ReasonMap) *Handlers {
	return &Handlers{Repo: repo, Reason: reason}
}
