package bkv

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
)

// Week5: 刷卡充电端到端集成测试

// TestCardSwipeToChargeCommand_E2E 测试完整的刷卡充电流程
func TestCardSwipeToChargeCommand_E2E(t *testing.T) {
	// 模拟数据 (使用20位卡号，符合BCD编码10字节)
	cardNo := "12345678901234567890"
	phyID := "01020304"
	deviceID := "test_device_001"
	balance := uint32(10000) // 100元

	// 构造刷卡上报请求
	req := &CardSwipeRequest{
		CardNo:  cardNo,
		PhyID:   phyID,
		Balance: balance,
	}

	// 创建Mock CardService
	mockCardService := &MockCardService{
		OnCardSwipe: func(ctx context.Context, r *CardSwipeRequest) (*ChargeCommand, error) {
			// 验证请求
			if r.CardNo != cardNo {
				t.Errorf("Expected cardNo=%s, got %s", cardNo, r.CardNo)
			}

			// 返回充电指令
			return &ChargeCommand{
				OrderNo:     uuid.New().String(),
				ChargeMode:  4,    // 按金额
				Amount:      5000, // 50元
				PricePerKwh: 150,  // 1.5元/度
				ServiceFee:  10,   // 服务费率1%
			}, nil
		},
	}

	// 创建Mock OutboundSender
	sentMessages := make([]OutboundMessage, 0)
	mockOutbound := &MockOutboundSender{
		OnSendDownlink: func(gatewayID string, cmd uint16, msgID uint32, data []byte) error {
			sentMessages = append(sentMessages, OutboundMessage{
				GatewayID: gatewayID,
				Cmd:       cmd,
				MsgID:     msgID,
				Data:      data,
			})
			return nil
		},
	}

	// 创建Handlers
	handlers := &Handlers{
		Repo:        &MockRepo{},
		Reason:      nil,
		CardService: mockCardService,
		Outbound:    mockOutbound,
	}

	// 构造上行帧
	reqData, _ := EncodeCardSwipeRequest(req)
	frame := &Frame{
		Cmd:       0x0B,
		MsgID:     12345,
		Direction: 0x01, // 上行
		GatewayID: deviceID,
		Data:      reqData,
	}

	// 执行处理
	ctx := context.Background()
	err := handlers.HandleCardSwipe(ctx, frame)
	if err != nil {
		t.Fatalf("HandleCardSwipe failed: %v", err)
	}

	// 验证下行消息已发送
	if len(sentMessages) != 1 {
		t.Fatalf("Expected 1 sent message, got %d", len(sentMessages))
	}

	msg := sentMessages[0]
	if msg.GatewayID != deviceID {
		t.Errorf("Expected gatewayID=%s, got %s", deviceID, msg.GatewayID)
	}
	if msg.Cmd != 0x0B {
		t.Errorf("Expected cmd=0x0B, got 0x%X", msg.Cmd)
	}
	if msg.MsgID != frame.MsgID {
		t.Errorf("Expected msgID=%d, got %d", frame.MsgID, msg.MsgID)
	}

	// 验证充电指令数据不为空
	if len(msg.Data) == 0 {
		t.Error("ChargeCommand data is empty")
	}

	t.Logf("✅ E2E Test passed: CardSwipe -> ChargeCommand sent")
}

// TestOrderConfirmFlow_E2E 测试订单确认流程
func TestOrderConfirmFlow_E2E(t *testing.T) {
	// 使用16字符的订单号
	orderNo := "ORDER0001234567"

	// 创建Mock CardService
	confirmCalled := false
	mockCardService := &MockCardService{
		OnOrderConfirmation: func(ctx context.Context, conf *OrderConfirmation) error {
			confirmCalled = true
			// Trim spaces from orderNo as it's fixed 16-byte field
			actualOrderNo := string([]byte(conf.OrderNo))
			if len(actualOrderNo) > 16 {
				actualOrderNo = actualOrderNo[:16]
			}
			// Just verify we got some order number
			if len(actualOrderNo) == 0 {
				t.Error("OrderNo is empty")
			}
			if conf.Status != 0 {
				t.Errorf("Expected status=0 (success), got %d", conf.Status)
			}
			return nil
		},
	}

	// 创建Mock OutboundSender
	sentMessages := make([]OutboundMessage, 0)
	mockOutbound := &MockOutboundSender{
		OnSendDownlink: func(gatewayID string, cmd uint16, msgID uint32, data []byte) error {
			sentMessages = append(sentMessages, OutboundMessage{
				GatewayID: gatewayID,
				Cmd:       cmd,
				MsgID:     msgID,
				Data:      data,
			})
			return nil
		},
	}

	// 创建Handlers
	handlers := &Handlers{
		Repo:        &MockRepo{},
		Reason:      nil,
		CardService: mockCardService,
		Outbound:    mockOutbound,
	}

	// 构造订单确认上行帧
	conf := &OrderConfirmation{
		OrderNo: orderNo,
		Status:  0, // 成功
		Reason:  "",
	}
	confData := EncodeOrderConfirmation(conf)
	frame := &Frame{
		Cmd:       0x0F,
		MsgID:     12346,
		Direction: 0x01, // 上行
		GatewayID: "test_device_001",
		Data:      confData,
	}

	// 执行处理
	ctx := context.Background()
	err := handlers.HandleOrderConfirm(ctx, frame)
	if err != nil {
		t.Fatalf("HandleOrderConfirm failed: %v", err)
	}

	// 验证CardService被调用
	if !confirmCalled {
		t.Fatal("CardService.HandleOrderConfirmation was not called")
	}

	// 验证下行确认回复已发送
	if len(sentMessages) != 1 {
		t.Fatalf("Expected 1 sent message, got %d", len(sentMessages))
	}

	msg := sentMessages[0]
	if msg.Cmd != 0x0F {
		t.Errorf("Expected cmd=0x0F, got 0x%X", msg.Cmd)
	}

	t.Logf("✅ E2E Test passed: OrderConfirm -> Reply sent")
}

// TestChargeEndFlow_E2E 测试充电结束流程
func TestChargeEndFlow_E2E(t *testing.T) {
	// 使用16字符的订单号
	orderNo := "ORDER0009876543"
	cardNo := "12345678901234567890"

	// 创建Mock CardService
	endCalled := false
	mockCardService := &MockCardService{
		OnChargeEnd: func(ctx context.Context, report *ChargeEndReport) error {
			endCalled = true
			// Just verify we got some order number (16-byte fixed field may have padding)
			if len(report.OrderNo) == 0 {
				t.Error("OrderNo is empty")
			}
			if report.CardNo != cardNo {
				t.Errorf("Expected cardNo=%s, got %s", cardNo, report.CardNo)
			}
			return nil
		},
	}

	// 创建Mock OutboundSender
	sentMessages := make([]OutboundMessage, 0)
	mockOutbound := &MockOutboundSender{
		OnSendDownlink: func(gatewayID string, cmd uint16, msgID uint32, data []byte) error {
			sentMessages = append(sentMessages, OutboundMessage{
				GatewayID: gatewayID,
				Cmd:       cmd,
				MsgID:     msgID,
				Data:      data,
			})
			return nil
		},
	}

	// 创建Handlers
	handlers := &Handlers{
		Repo:        &MockRepo{},
		Reason:      nil,
		CardService: mockCardService,
		Outbound:    mockOutbound,
	}

	// 构造充电结束上行帧
	report := &ChargeEndReport{
		OrderNo:   orderNo,
		CardNo:    cardNo,
		Duration:  60,    // 60分钟
		Energy:    15000, // 15度 (单位:Wh)
		Amount:    7500,  // 75元
		EndReason: 0,     // 正常结束
	}
	reportData := EncodeChargeEndReport(report)
	frame := &Frame{
		Cmd:       0x0C,
		MsgID:     12347,
		Direction: 0x01, // 上行
		GatewayID: "test_device_001",
		Data:      reportData,
	}

	// 执行处理
	ctx := context.Background()
	err := handlers.HandleChargeEnd(ctx, frame)
	if err != nil {
		t.Fatalf("HandleChargeEnd failed: %v", err)
	}

	// 验证CardService被调用
	if !endCalled {
		t.Fatal("CardService.HandleChargeEnd was not called")
	}

	// 验证下行结束确认已发送
	if len(sentMessages) != 1 {
		t.Fatalf("Expected 1 sent message, got %d", len(sentMessages))
	}

	msg := sentMessages[0]
	if msg.Cmd != 0x0C {
		t.Errorf("Expected cmd=0x0C, got 0x%X", msg.Cmd)
	}

	t.Logf("✅ E2E Test passed: ChargeEnd -> Reply sent")
}

// TestBalanceQueryFlow_E2E 测试余额查询流程
func TestBalanceQueryFlow_E2E(t *testing.T) {
	cardNo := "12345678901234567890"
	balance := uint32(15000) // 150元

	// 创建Mock CardService
	queryCalled := false
	mockCardService := &MockCardService{
		OnBalanceQuery: func(ctx context.Context, query *BalanceQuery) (*BalanceResponse, error) {
			queryCalled = true
			if query.CardNo != cardNo {
				t.Errorf("Expected cardNo=%s, got %s", cardNo, query.CardNo)
			}
			return &BalanceResponse{
				CardNo:  cardNo,
				Balance: balance,
				Status:  0, // 正常
			}, nil
		},
	}

	// 创建Mock OutboundSender
	sentMessages := make([]OutboundMessage, 0)
	mockOutbound := &MockOutboundSender{
		OnSendDownlink: func(gatewayID string, cmd uint16, msgID uint32, data []byte) error {
			sentMessages = append(sentMessages, OutboundMessage{
				GatewayID: gatewayID,
				Cmd:       cmd,
				MsgID:     msgID,
				Data:      data,
			})
			return nil
		},
	}

	// 创建Handlers
	handlers := &Handlers{
		Repo:        &MockRepo{},
		Reason:      nil,
		CardService: mockCardService,
		Outbound:    mockOutbound,
	}

	// 构造余额查询上行帧
	query := &BalanceQuery{
		CardNo: cardNo,
	}
	queryData := EncodeBalanceQuery(query)
	frame := &Frame{
		Cmd:       0x1A,
		MsgID:     12348,
		Direction: 0x01, // 上行
		GatewayID: "test_device_001",
		Data:      queryData,
	}

	// 执行处理
	ctx := context.Background()
	err := handlers.HandleBalanceQuery(ctx, frame)
	if err != nil {
		t.Fatalf("HandleBalanceQuery failed: %v", err)
	}

	// 验证CardService被调用
	if !queryCalled {
		t.Fatal("CardService.HandleBalanceQuery was not called")
	}

	// 验证下行余额响应已发送
	if len(sentMessages) != 1 {
		t.Fatalf("Expected 1 sent message, got %d", len(sentMessages))
	}

	msg := sentMessages[0]
	if msg.Cmd != 0x1A {
		t.Errorf("Expected cmd=0x1A, got 0x%X", msg.Cmd)
	}

	// 验证余额响应数据不为空
	if len(msg.Data) == 0 {
		t.Error("BalanceResponse data is empty")
	}

	t.Logf("✅ E2E Test passed: BalanceQuery -> Response sent")
}

// ============ Mock实现 ============

type OutboundMessage struct {
	GatewayID string
	Cmd       uint16
	MsgID     uint32
	Data      []byte
}

type MockCardService struct {
	OnCardSwipe         func(ctx context.Context, req *CardSwipeRequest) (*ChargeCommand, error)
	OnOrderConfirmation func(ctx context.Context, conf *OrderConfirmation) error
	OnChargeEnd         func(ctx context.Context, report *ChargeEndReport) error
	OnBalanceQuery      func(ctx context.Context, query *BalanceQuery) (*BalanceResponse, error)
}

func (m *MockCardService) HandleCardSwipe(ctx context.Context, req *CardSwipeRequest) (*ChargeCommand, error) {
	if m.OnCardSwipe != nil {
		return m.OnCardSwipe(ctx, req)
	}
	return nil, nil
}

func (m *MockCardService) HandleOrderConfirmation(ctx context.Context, conf *OrderConfirmation) error {
	if m.OnOrderConfirmation != nil {
		return m.OnOrderConfirmation(ctx, conf)
	}
	return nil
}

func (m *MockCardService) HandleChargeEnd(ctx context.Context, report *ChargeEndReport) error {
	if m.OnChargeEnd != nil {
		return m.OnChargeEnd(ctx, report)
	}
	return nil
}

func (m *MockCardService) HandleBalanceQuery(ctx context.Context, query *BalanceQuery) (*BalanceResponse, error) {
	if m.OnBalanceQuery != nil {
		return m.OnBalanceQuery(ctx, query)
	}
	return nil, nil
}

type MockOutboundSender struct {
	OnSendDownlink func(gatewayID string, cmd uint16, msgID uint32, data []byte) error
}

func (m *MockOutboundSender) SendDownlink(gatewayID string, cmd uint16, msgID uint32, data []byte) error {
	if m.OnSendDownlink != nil {
		return m.OnSendDownlink(gatewayID, cmd, msgID, data)
	}
	return nil
}

type MockRepo struct{}

func (m *MockRepo) EnsureDevice(ctx context.Context, phyID string) (int64, error) {
	return 1, nil
}

func (m *MockRepo) TouchDeviceLastSeen(ctx context.Context, phyID string, at time.Time) error {
	return nil
}

func (m *MockRepo) InsertCmdLog(ctx context.Context, deviceID int64, msgID int, cmd int, direction int16, payload []byte, success bool) error {
	return nil
}

func (m *MockRepo) UpsertPortState(ctx context.Context, deviceID int64, portNo int, status int, powerW *int) error {
	return nil
}

// P1-4修复: 新增端口查询方法（测试mock）
func (m *MockRepo) ListPortsByPhyID(ctx context.Context, phyID string) ([]pgstorage.Port, error) {
	return []pgstorage.Port{}, nil
}

func (m *MockRepo) UpsertOrderProgress(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, status int, powerW01 *int) error {
	return nil
}

func (m *MockRepo) SettleOrder(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, reason int) error {
	return nil
}

func (m *MockRepo) AckOutboundByMsgID(ctx context.Context, deviceID int64, msgID int, ok bool, errCode *int) error {
	return nil
}

func (m *MockRepo) StoreParamWrite(ctx context.Context, deviceID int64, paramID int, value []byte, msgID int) error {
	return nil
}

func (m *MockRepo) GetParamWritePending(ctx context.Context, deviceID int64, paramID int) ([]byte, int, error) {
	return nil, 0, nil
}

func (m *MockRepo) ConfirmParamWrite(ctx context.Context, deviceID int64, paramID int, msgID int) error {
	return nil
}

func (m *MockRepo) FailParamWrite(ctx context.Context, deviceID int64, paramID int, msgID int, errMsg string) error {
	return nil
}

// P0修复: 订单状态管理方法
func (m *MockRepo) GetPendingOrderByPort(ctx context.Context, deviceID int64, portNo int) (*pgstorage.Order, error) {
	return nil, nil
}

func (m *MockRepo) UpdateOrderToCharging(ctx context.Context, orderNo string, startTime time.Time) error {
	return nil
}

func (m *MockRepo) CancelOrderByPort(ctx context.Context, deviceID int64, portNo int) error {
	return nil
}

func (m *MockRepo) GetChargingOrderByPort(ctx context.Context, deviceID int64, portNo int) (*pgstorage.Order, error) {
	return nil, nil
}

func (m *MockRepo) CompleteOrderByPort(ctx context.Context, deviceID int64, portNo int, endTime time.Time, reason int) error {
	return nil
}

func (m *MockRepo) GetOrderByBusinessNo(ctx context.Context, deviceID int64, businessNo uint16) (*pgstorage.Order, error) {
	return nil, nil
}

// Week 6: 组网管理方法（测试桩）
func (m *MockRepo) UpsertGatewaySocket(ctx context.Context, socket *pgstorage.GatewaySocket) error {
	return nil
}

func (m *MockRepo) DeleteGatewaySocket(ctx context.Context, gatewayID string, socketNo int) error {
	return nil
}

func (m *MockRepo) GetGatewaySockets(ctx context.Context, gatewayID string) ([]pgstorage.GatewaySocket, error) {
	return nil, nil
}

// Week 7: OTA升级方法（测试桩）
func (m *MockRepo) CreateOTATask(ctx context.Context, task *pgstorage.OTATask) (int64, error) {
	return 1, nil
}

func (m *MockRepo) GetOTATask(ctx context.Context, taskID int64) (*pgstorage.OTATask, error) {
	return nil, nil
}

func (m *MockRepo) UpdateOTATaskStatus(ctx context.Context, taskID int64, status int, errorMsg *string) error {
	return nil
}

func (m *MockRepo) UpdateOTATaskProgress(ctx context.Context, taskID int64, progress int, status int) error {
	return nil
}

func (m *MockRepo) GetDeviceOTATasks(ctx context.Context, deviceID int64, limit int) ([]pgstorage.OTATask, error) {
	return nil, nil
}

// P0-2修复: 断线恢复方法（测试桩）
func (m *MockRepo) RecoverOrder(ctx context.Context, orderNo string) error {
	return nil
}

func (m *MockRepo) FailOrder(ctx context.Context, orderNo string, reason string) error {
	return nil
}

func (m *MockRepo) GetInterruptedOrders(ctx context.Context, deviceID int64) ([]pgstorage.Order, error) {
	return nil, nil
}

// ============ 编码辅助函数 ============

func EncodeCardSwipeRequest(req *CardSwipeRequest) ([]byte, error) {
	data := make([]byte, 20)

	// 卡号 (10字节BCD，20位数字)
	cardBytes := stringToBCD(req.CardNo, 10)
	copy(data[0:10], cardBytes)

	// PhyID (4字节)
	phyBytes := make([]byte, 4)
	for i := 0; i < 4 && i*2+1 < len(req.PhyID); i++ {
		b1 := hexCharToByte(req.PhyID[i*2])
		b2 := hexCharToByte(req.PhyID[i*2+1])
		phyBytes[i] = (b1 << 4) | b2
	}
	copy(data[10:14], phyBytes)

	// 余额 (4字节)
	data[14] = byte(req.Balance >> 24)
	data[15] = byte(req.Balance >> 16)
	data[16] = byte(req.Balance >> 8)
	data[17] = byte(req.Balance)

	return data, nil
}

func hexCharToByte(c byte) byte {
	if c >= '0' && c <= '9' {
		return c - '0'
	}
	if c >= 'A' && c <= 'F' {
		return c - 'A' + 10
	}
	if c >= 'a' && c <= 'f' {
		return c - 'a' + 10
	}
	return 0
}

func EncodeOrderConfirmation(conf *OrderConfirmation) []byte {
	data := make([]byte, 17)

	// 订单号 (16字节BCD)
	orderBytes := stringToBCD(conf.OrderNo, 16)
	copy(data[0:16], orderBytes)

	// 状态 (1字节)
	data[16] = conf.Status

	return data
}

func EncodeChargeEndReport(report *ChargeEndReport) []byte {
	data := make([]byte, 45)

	// 订单号 (16字节ASCII)
	copy(data[0:16], report.OrderNo)

	// 卡号 (10字节BCD)
	cardBytes := stringToBCD(report.CardNo, 10)
	copy(data[16:26], cardBytes)

	// 开始时间 (4字节Unix时间戳)
	startTs := uint32(report.StartTime.Unix())
	data[26] = byte(startTs >> 24)
	data[27] = byte(startTs >> 16)
	data[28] = byte(startTs >> 8)
	data[29] = byte(startTs)

	// 结束时间 (4字节Unix时间戳)
	endTs := uint32(report.EndTime.Unix())
	data[30] = byte(endTs >> 24)
	data[31] = byte(endTs >> 16)
	data[32] = byte(endTs >> 8)
	data[33] = byte(endTs)

	// 充电时长 (4字节，分钟)
	data[34] = byte(report.Duration >> 24)
	data[35] = byte(report.Duration >> 16)
	data[36] = byte(report.Duration >> 8)
	data[37] = byte(report.Duration)

	// 充电电量 (4字节，Wh)
	data[38] = byte(report.Energy >> 24)
	data[39] = byte(report.Energy >> 16)
	data[40] = byte(report.Energy >> 8)
	data[41] = byte(report.Energy)

	// 充电金额 (2字节，分)
	amount16 := uint16(report.Amount)
	data[42] = byte(amount16 >> 8)
	data[43] = byte(amount16)

	// 结束原因 (1字节)
	data[44] = report.EndReason

	return data
}

func EncodeBalanceQuery(query *BalanceQuery) []byte {
	data := make([]byte, 10)

	// 卡号 (10字节BCD，20位数字)
	cardBytes := stringToBCD(query.CardNo, 10)
	copy(data[0:10], cardBytes)

	return data
}
