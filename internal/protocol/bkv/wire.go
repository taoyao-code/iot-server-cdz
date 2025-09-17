package bkv

import (
	"context"
	"fmt"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
)

// repoAdapter 适配器，为 pg.Repository 提供完整的 repoAPI 实现
type repoAdapter struct {
	*pgstorage.Repository
	// 简单的内存存储用于参数校验（生产环境应该使用数据库）
	paramStore map[string]paramEntry
}

type paramEntry struct {
	Value []byte
	MsgID int
}

func (r *repoAdapter) StoreParamWrite(ctx context.Context, deviceID int64, paramID int, value []byte, msgID int) error {
	if r.paramStore == nil {
		r.paramStore = make(map[string]paramEntry)
	}
	key := fmt.Sprintf("%d:%d", deviceID, paramID)
	r.paramStore[key] = paramEntry{Value: value, MsgID: msgID}
	return nil
}

func (r *repoAdapter) GetParamWritePending(ctx context.Context, deviceID int64, paramID int) ([]byte, int, error) {
	if r.paramStore == nil {
		return nil, 0, fmt.Errorf("no pending param")
	}
	key := fmt.Sprintf("%d:%d", deviceID, paramID)
	if entry, exists := r.paramStore[key]; exists {
		return entry.Value, entry.MsgID, nil
	}
	return nil, 0, fmt.Errorf("no pending param for %s", key)
}

// NewHandlers 构造 BKV 处理集合
func NewHandlers(repo *pgstorage.Repository, reason *ReasonMap) *Handlers {
	adapter := &repoAdapter{Repository: repo}
	return &Handlers{Repo: adapter, Reason: reason}
}
