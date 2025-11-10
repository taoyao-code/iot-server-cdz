package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/taoyao-code/iot-server/internal/protocol/bkv"
	"github.com/taoyao-code/iot-server/internal/storage/pg"
)

// MockRepository 模拟Repository用于测试
type MockRepository struct {
	GetTransactionFunc                     func(ctx context.Context, orderNo string) (*pg.CardTransaction, error)
	UpdateTransactionChargingWithEventFunc func(ctx context.Context, orderNo string, eventData []byte) error
	FailTransactionWithEventFunc           func(ctx context.Context, orderNo, reason string, eventData []byte) error
}

func (m *MockRepository) GetTransaction(ctx context.Context, orderNo string) (*pg.CardTransaction, error) {
	if m.GetTransactionFunc != nil {
		return m.GetTransactionFunc(ctx, orderNo)
	}
	return nil, nil
}

func (m *MockRepository) UpdateTransactionChargingWithEvent(ctx context.Context, orderNo string, eventData []byte) error {
	if m.UpdateTransactionChargingWithEventFunc != nil {
		return m.UpdateTransactionChargingWithEventFunc(ctx, orderNo, eventData)
	}
	return nil
}

func (m *MockRepository) FailTransactionWithEvent(ctx context.Context, orderNo, reason string, eventData []byte) error {
	if m.FailTransactionWithEventFunc != nil {
		return m.FailTransactionWithEventFunc(ctx, orderNo, reason, eventData)
	}
	return nil
}

// 实现CardRepositoryAPI接口的其他方法（测试中未使用，返回nil）
func (m *MockRepository) GetCard(ctx context.Context, cardNo string) (*pg.Card, error) {
	return nil, nil
}

func (m *MockRepository) CreateTransaction(ctx context.Context, tx *pg.CardTransaction) (*pg.CardTransaction, error) {
	return nil, nil
}

func (m *MockRepository) UpdateCardBalance(ctx context.Context, cardNo string, amount float64, changeType, description string) error {
	return nil
}

func (m *MockRepository) CompleteTransaction(ctx context.Context, orderNo string, energyKwh, totalAmount float64) error {
	return nil
}

func (m *MockRepository) GetCardTransactions(ctx context.Context, cardNo string, limit int) ([]pg.CardTransaction, error) {
	return nil, nil
}

func (m *MockRepository) CreateCard(ctx context.Context, cardNo string, balance float64, status string) (*pg.Card, error) {
	return nil, nil
}

func (m *MockRepository) GetNextSequenceNo(ctx context.Context, orderNo string) (int, error) {
	return 0, nil
}

func (m *MockRepository) InsertEvent(ctx context.Context, orderNo, eventType string, eventData []byte, sequenceNo int) error {
	return nil
}

// TestHandleOrderConfirmation_Success 测试正常ACK处理
func TestHandleOrderConfirmation_Success(t *testing.T) {
	mockRepo := &MockRepository{
		GetTransactionFunc: func(ctx context.Context, orderNo string) (*pg.CardTransaction, error) {
			return &pg.CardTransaction{
				OrderNo:   "TEST001",
				Status:    "pending",
				CreatedAt: time.Now().Add(-5 * time.Second), // 5秒前创建
			}, nil
		},
		UpdateTransactionChargingWithEventFunc: func(ctx context.Context, orderNo string, eventData []byte) error {
			assert.Equal(t, "TEST001", orderNo)
			return nil
		},
	}

	logger := zap.NewNop()
	svc := NewCardService(mockRepo, nil, logger)

	conf := &bkv.OrderConfirmation{
		OrderNo: "TEST001",
		Status:  0, // 成功
	}

	err := svc.HandleOrderConfirmation(context.Background(), conf)
	assert.NoError(t, err)
}

// TestHandleOrderConfirmation_InvalidStatus 测试无效状态拒绝
func TestHandleOrderConfirmation_InvalidStatus(t *testing.T) {
	testCases := []struct {
		name   string
		status string
	}{
		{"timeout", "timeout"},
		{"charging", "charging"},
		{"completed", "completed"},
		{"failed", "failed"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockRepo := &MockRepository{
				GetTransactionFunc: func(ctx context.Context, orderNo string) (*pg.CardTransaction, error) {
					return &pg.CardTransaction{
						OrderNo:   "TEST002",
						Status:    tc.status, // 非pending状态
						CreatedAt: time.Now(),
					}, nil
				},
			}

			logger := zap.NewNop()
			svc := NewCardService(mockRepo, nil, logger)

			conf := &bkv.OrderConfirmation{
				OrderNo: "TEST002",
				Status:  0,
			}

			err := svc.HandleOrderConfirmation(context.Background(), conf)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "P1-2: invalid order status")
			assert.Contains(t, err.Error(), "expected=pending")
			assert.Contains(t, err.Error(), tc.status)
		})
	}
}

// TestHandleOrderConfirmation_Timeout 测试超时ACK拒绝
func TestHandleOrderConfirmation_Timeout(t *testing.T) {
	testCases := []struct {
		name       string
		ageSeconds int
		shouldFail bool
	}{
		{"8秒内ACK-应成功", 8, false},
		{"9秒内ACK-应成功", 9, false},
		{"11秒后ACK-应拒绝", 11, true},
		{"15秒后ACK-应拒绝", 15, true},
		{"60秒后ACK-应拒绝", 60, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			oldTime := time.Now().Add(-time.Duration(tc.ageSeconds) * time.Second)

			mockRepo := &MockRepository{
				GetTransactionFunc: func(ctx context.Context, orderNo string) (*pg.CardTransaction, error) {
					return &pg.CardTransaction{
						OrderNo:   "TEST003",
						Status:    "pending",
						CreatedAt: oldTime,
					}, nil
				},
				UpdateTransactionChargingWithEventFunc: func(ctx context.Context, orderNo string, eventData []byte) error {
					return nil
				},
			}

			logger := zap.NewNop()
			svc := NewCardService(mockRepo, nil, logger)

			conf := &bkv.OrderConfirmation{
				OrderNo: "TEST003",
				Status:  0,
			}

			err := svc.HandleOrderConfirmation(context.Background(), conf)

			if tc.shouldFail {
				require.Error(t, err, "ACK在%d秒后应被拒绝", tc.ageSeconds)
				assert.Contains(t, err.Error(), "P1-2: ACK timeout")
			} else {
				assert.NoError(t, err, "ACK在%d秒内应被接受", tc.ageSeconds)
			}
		})
	}
}

// TestHandleOrderConfirmation_DeviceRejected 测试设备拒绝订单
func TestHandleOrderConfirmation_DeviceRejected(t *testing.T) {
	mockRepo := &MockRepository{
		GetTransactionFunc: func(ctx context.Context, orderNo string) (*pg.CardTransaction, error) {
			return &pg.CardTransaction{
				OrderNo:   "TEST004",
				Status:    "pending",
				CreatedAt: time.Now().Add(-3 * time.Second),
			}, nil
		},
		FailTransactionWithEventFunc: func(ctx context.Context, orderNo, reason string, eventData []byte) error {
			assert.Equal(t, "TEST004", orderNo)
			assert.Equal(t, "端口故障", reason)
			return nil
		},
	}

	logger := zap.NewNop()
	svc := NewCardService(mockRepo, nil, logger)

	conf := &bkv.OrderConfirmation{
		OrderNo: "TEST004",
		Status:  1, // 失败
		Reason:  "端口故障",
	}

	err := svc.HandleOrderConfirmation(context.Background(), conf)
	assert.NoError(t, err)
}

// TestHandleOrderConfirmation_LoggerNil 测试logger为nil时不崩溃
func TestHandleOrderConfirmation_LoggerNil(t *testing.T) {
	mockRepo := &MockRepository{
		GetTransactionFunc: func(ctx context.Context, orderNo string) (*pg.CardTransaction, error) {
			return &pg.CardTransaction{
				OrderNo:   "TEST005",
				Status:    "timeout", // 无效状态，会触发日志
				CreatedAt: time.Now(),
			}, nil
		},
	}

	// logger为nil
	svc := NewCardService(mockRepo, nil, nil)

	conf := &bkv.OrderConfirmation{
		OrderNo: "TEST005",
		Status:  0,
	}

	// 应该返回错误但不崩溃
	err := svc.HandleOrderConfirmation(context.Background(), conf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "P1-2: invalid order status")
}
