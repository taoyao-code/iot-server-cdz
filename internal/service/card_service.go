package service

import (
	"context"
	"fmt"
	"time"

	"github.com/taoyao-code/iot-server/internal/protocol/bkv"
	"github.com/taoyao-code/iot-server/internal/storage/pg"
)

const (
	// P1-2修复: ACK超时配置
	OrderACKTimeout = 10 * time.Second // 订单确认ACK超时时间
)

// CardService 刷卡充电业务服务
type CardService struct {
	repo    *pg.Repository
	pricing *PricingEngine
}

// NewCardService 创建卡片服务
func NewCardService(repo *pg.Repository, pricing *PricingEngine) *CardService {
	return &CardService{
		repo:    repo,
		pricing: pricing,
	}
}

// HandleCardSwipe 处理刷卡事件
func (s *CardService) HandleCardSwipe(ctx context.Context, req *bkv.CardSwipeRequest) (*bkv.ChargeCommand, error) {
	// 1. 查询卡片信息
	card, err := s.repo.GetCard(ctx, req.CardNo)
	if err != nil {
		return nil, fmt.Errorf("卡片不存在: %w", err)
	}

	// 2. 验证卡片状态
	if card.Status != "active" {
		return nil, fmt.Errorf("卡片状态无效: %s", card.Status)
	}

	// 3. 检查余额
	if card.Balance <= 0 {
		return nil, fmt.Errorf("卡片余额不足: %.2f元", card.Balance)
	}

	// 4. 生成订单号
	orderNo := bkv.GenerateOrderNo(req.CardNo, req.PhyID)

	// 5. 计算充电参数（默认按时长模式）
	chargeMode := 1 // 按时长
	maxAmount := card.Balance
	if maxAmount > 50 {
		maxAmount = 50 // 单次最多消费50元
	}

	// 根据余额计算充电时长（假设每小时2元）
	duration := uint32(maxAmount / 2.0 * 60) // 分钟

	// 6. 创建交易记录
	amountFloat := maxAmount
	durationInt := int(duration)
	tx := &pg.CardTransaction{
		CardNo:          req.CardNo,
		DeviceID:        "DEV-" + req.PhyID,
		PhyID:           req.PhyID,
		OrderNo:         orderNo,
		ChargeMode:      chargeMode,
		Amount:          &amountFloat,
		DurationMinutes: &durationInt,
		Status:          "pending",
	}

	_, err = s.repo.CreateTransaction(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("创建订单失败: %w", err)
	}

	// 7. 构造充电指令
	cmd := &bkv.ChargeCommand{
		OrderNo:     orderNo,
		ChargeMode:  uint8(chargeMode),
		Amount:      uint32(maxAmount * 100), // 元转分
		Duration:    duration,
		Power:       2000, // 2000W
		PricePerKwh: 50,   // 0.5元/度
		ServiceFee:  50,   // 5%服务费
	}

	return cmd, nil
}

// HandleOrderConfirmation 处理订单确认
func (s *CardService) HandleOrderConfirmation(ctx context.Context, conf *bkv.OrderConfirmation) error {
	// P1-2修复: 检查订单状态和时效性
	tx, err := s.repo.GetTransaction(ctx, conf.OrderNo)
	if err != nil {
		return fmt.Errorf("订单不存在: %w", err)
	}

	// 1. 检查订单状态必须为pending
	if tx.Status != "pending" {
		return fmt.Errorf("P1-2: invalid order status for ACK, expected=pending, actual=%s, order_no=%s", 
			tx.Status, conf.OrderNo)
	}

	// 2. 检查ACK时效性（创建时间超过OrderACKTimeout拒绝处理）
	if time.Since(tx.CreatedAt) > OrderACKTimeout {
		return fmt.Errorf("P1-2: ACK timeout, order created at %s, timeout=%v, order_no=%s", 
			tx.CreatedAt.Format(time.RFC3339), OrderACKTimeout, conf.OrderNo)
	}

	// 更新订单状态
	if conf.Status == 0 {
		// 设备接受订单，更新为充电中
		return s.repo.UpdateTransactionCharging(ctx, conf.OrderNo)
	}

	// 设备拒绝订单，标记失败
	reason := conf.Reason
	if reason == "" {
		reason = "设备拒绝"
	}
	return s.repo.FailTransaction(ctx, conf.OrderNo, reason)
}

// HandleChargeEnd 处理充电结束
func (s *CardService) HandleChargeEnd(ctx context.Context, report *bkv.ChargeEndReport) error {
	// 1. 查询订单
	tx, err := s.repo.GetTransaction(ctx, report.OrderNo)
	if err != nil {
		return fmt.Errorf("订单不存在: %w", err)
	}

	// 2. 计算实际消费金额
	energyKwh := float64(report.Energy) / 1000.0   // Wh转Kwh
	actualAmount := float64(report.Amount) / 100.0 // 分转元

	// 3. 完成订单
	err = s.repo.CompleteTransaction(ctx, report.OrderNo, energyKwh, actualAmount)
	if err != nil {
		return fmt.Errorf("完成订单失败: %w", err)
	}

	// 4. 扣除余额
	err = s.repo.UpdateCardBalance(ctx, tx.CardNo, -actualAmount, "consume",
		fmt.Sprintf("充电消费 订单:%s", report.OrderNo))
	if err != nil {
		return fmt.Errorf("扣款失败: %w", err)
	}

	return nil
}

// HandleBalanceQuery 处理余额查询
func (s *CardService) HandleBalanceQuery(ctx context.Context, query *bkv.BalanceQuery) (*bkv.BalanceResponse, error) {
	// 查询卡片
	card, err := s.repo.GetCard(ctx, query.CardNo)
	if err != nil {
		// 卡片不存在
		return &bkv.BalanceResponse{
			CardNo:  query.CardNo,
			Balance: 0,
			Status:  1, // 无效
		}, nil
	}

	// 返回余额
	status := uint8(0) // 正常
	if card.Status != "active" {
		status = 2 // 冻结
	}

	return &bkv.BalanceResponse{
		CardNo:  query.CardNo,
		Balance: uint32(card.Balance * 100), // 元转分
		Status:  status,
	}, nil
}

// GetCardTransactions 查询卡片交易历史
func (s *CardService) GetCardTransactions(ctx context.Context, cardNo string, limit int) ([]pg.CardTransaction, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.repo.GetCardTransactions(ctx, cardNo, limit)
}

// CreateCard 创建新卡片
func (s *CardService) CreateCard(ctx context.Context, cardNo string, initialBalance float64) (*pg.Card, error) {
	// 创建卡片
	card, err := s.repo.CreateCard(ctx, cardNo, initialBalance, "active")
	if err != nil {
		return nil, fmt.Errorf("创建卡片失败: %w", err)
	}

	// 记录初始充值日志
	if initialBalance > 0 {
		err = s.repo.UpdateCardBalance(ctx, cardNo, initialBalance, "recharge", "初始充值")
		if err != nil {
			return nil, fmt.Errorf("记录充值失败: %w", err)
		}
	}

	return card, nil
}

// RechargeCard 卡片充值
func (s *CardService) RechargeCard(ctx context.Context, cardNo string, amount float64) error {
	if amount <= 0 {
		return fmt.Errorf("充值金额必须大于0")
	}

	// 验证卡片存在
	_, err := s.repo.GetCard(ctx, cardNo)
	if err != nil {
		return fmt.Errorf("卡片不存在: %w", err)
	}

	// 充值
	return s.repo.UpdateCardBalance(ctx, cardNo, amount, "recharge",
		fmt.Sprintf("充值 %.2f元 时间:%s", amount, time.Now().Format("2006-01-02 15:04:05")))
}
