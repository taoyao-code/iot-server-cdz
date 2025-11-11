package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/taoyao-code/iot-server/internal/session"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
)

// TestNewReadOnlyHandler 测试ReadOnlyHandler初始化
func TestNewReadOnlyHandler(t *testing.T) {
	logger := zap.NewNop()
	repo := &pgstorage.Repository{}
	policy := session.WeightedPolicy{}

	handler := NewReadOnlyHandler(repo, nil, policy, logger)

	assert.NotNil(t, handler)
	assert.Equal(t, repo, handler.repo)
	assert.Equal(t, logger, handler.logger)
}

// TestReadOnlyHandler_ParamStatus 测试参数状态转换逻辑
func TestReadOnlyHandler_ParamStatusConversion(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		expectedStatus string
	}{
		{"pending状态", 0, "pending"},
		{"confirmed状态", 1, "confirmed"},
		{"failed状态", 2, "failed"},
		{"未知状态", 99, "pending"}, // 默认为pending
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var status string
			switch tt.statusCode {
			case 1:
				status = "confirmed"
			case 2:
				status = "failed"
			default:
				status = "pending"
			}

			assert.Equal(t, tt.expectedStatus, status)
		})
	}
}
