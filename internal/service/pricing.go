package service

import (
	"fmt"
	"math"
)

// PricingEngine 计费引擎
type PricingEngine struct {
	PricePerKwh     float64 // 电价（元/度）
	ServiceFeeRate  float64 // 服务费率（0-1）
	MinCharge       float64 // 最低消费（元）
	MaxSingleCharge float64 // 单次最高消费（元）
}

// NewPricingEngine 创建计费引擎
func NewPricingEngine() *PricingEngine {
	return &PricingEngine{
		PricePerKwh:     0.5,   // 0.5元/度
		ServiceFeeRate:  0.05,  // 5%服务费
		MinCharge:       1.0,   // 最低1元
		MaxSingleCharge: 100.0, // 单次最高100元
	}
}

// ChargeParams 充电参数
type ChargeParams struct {
	Mode     int     // 充电模式：1=按时长,2=按电量,3=按功率,4=充满
	Duration uint32  // 时长（分钟）
	Energy   float64 // 电量（度）
	Power    uint16  // 功率（瓦）
	Amount   float64 // 金额（元）
}

// CalculateByDuration 按时长计费
// 参数：duration(分钟), maxAmount(元), power(瓦)
func (p *PricingEngine) CalculateByDuration(duration int, maxAmount float64, power uint16) ChargeParams {
	// 预估电量：功率(W) * 时长(h)
	hours := float64(duration) / 60.0
	estimatedKwh := float64(power) * hours / 1000.0

	// 预估费用
	estimatedAmount := estimatedKwh * p.PricePerKwh * (1 + p.ServiceFeeRate)

	// 限制最大金额
	finalAmount := math.Min(estimatedAmount, maxAmount)
	finalAmount = math.Max(finalAmount, p.MinCharge)

	// 根据实际金额反推充电参数
	actualKwh := finalAmount / (p.PricePerKwh * (1 + p.ServiceFeeRate))
	actualDuration := uint32(actualKwh * 1000.0 / float64(power) * 60.0)

	return ChargeParams{
		Mode:     1,
		Duration: actualDuration,
		Energy:   actualKwh,
		Power:    power,
		Amount:   finalAmount,
	}
}

// CalculateByEnergy 按电量计费
// 参数：energy(度), maxAmount(元)
func (p *PricingEngine) CalculateByEnergy(energy float64, maxAmount float64) ChargeParams {
	// 计算费用
	amount := energy * p.PricePerKwh * (1 + p.ServiceFeeRate)

	// 限制最大金额
	finalAmount := math.Min(amount, maxAmount)
	finalAmount = math.Max(finalAmount, p.MinCharge)

	// 根据实际金额反推电量
	actualEnergy := finalAmount / (p.PricePerKwh * (1 + p.ServiceFeeRate))

	return ChargeParams{
		Mode:   2,
		Energy: actualEnergy,
		Amount: finalAmount,
	}
}

// CalculateByPower 按功率计费
// 参数：power(瓦), duration(分钟), maxAmount(元)
func (p *PricingEngine) CalculateByPower(power uint16, duration int, maxAmount float64) ChargeParams {
	// 预估电量
	hours := float64(duration) / 60.0
	estimatedKwh := float64(power) * hours / 1000.0

	// 计算费用
	amount := estimatedKwh * p.PricePerKwh * (1 + p.ServiceFeeRate)

	// 限制最大金额
	finalAmount := math.Min(amount, maxAmount)
	finalAmount = math.Max(finalAmount, p.MinCharge)

	return ChargeParams{
		Mode:     3,
		Duration: uint32(duration),
		Energy:   estimatedKwh,
		Power:    power,
		Amount:   finalAmount,
	}
}

// CalculateFull 充满自停计费
// 参数：maxAmount(元)
func (p *PricingEngine) CalculateFull(maxAmount float64) ChargeParams {
	// 充满自停模式，只设置最大金额
	finalAmount := math.Min(maxAmount, p.MaxSingleCharge)
	finalAmount = math.Max(finalAmount, p.MinCharge)

	return ChargeParams{
		Mode:   4,
		Amount: finalAmount,
	}
}

// CalculateActualCost 计算实际消费
// 根据实际充电电量计算费用
func (p *PricingEngine) CalculateActualCost(energyWh uint32) float64 {
	energyKwh := float64(energyWh) / 1000.0
	cost := energyKwh * p.PricePerKwh * (1 + p.ServiceFeeRate)
	return math.Max(cost, p.MinCharge)
}

// EstimateDuration 根据金额估算时长
// 返回分钟数
func (p *PricingEngine) EstimateDuration(amount float64, power uint16) int {
	// 扣除服务费后的电费
	energyCost := amount / (1 + p.ServiceFeeRate)

	// 可充电量（度）
	energyKwh := energyCost / p.PricePerKwh

	// 充电时长（小时）
	hours := energyKwh * 1000.0 / float64(power)

	return int(hours * 60.0)
}

// EstimateEnergy 根据金额估算电量
// 返回度数
func (p *PricingEngine) EstimateEnergy(amount float64) float64 {
	// 扣除服务费后的电费
	energyCost := amount / (1 + p.ServiceFeeRate)

	// 可充电量（度）
	return energyCost / p.PricePerKwh
}

// ValidateAmount 验证金额是否合法
func (p *PricingEngine) ValidateAmount(amount float64) error {
	if amount < p.MinCharge {
		return fmt.Errorf("金额不足最低消费: 需要%.2f元", p.MinCharge)
	}
	if amount > p.MaxSingleCharge {
		return fmt.Errorf("金额超过单次最高限额: 最多%.2f元", p.MaxSingleCharge)
	}
	return nil
}

// GetPriceInfo 获取计费信息
func (p *PricingEngine) GetPriceInfo() map[string]interface{} {
	return map[string]interface{}{
		"price_per_kwh":     p.PricePerKwh,
		"service_fee_rate":  p.ServiceFeeRate,
		"service_fee_pct":   p.ServiceFeeRate * 100,
		"min_charge":        p.MinCharge,
		"max_single_charge": p.MaxSingleCharge,
	}
}

// SetPricing 设置计费参数
func (p *PricingEngine) SetPricing(pricePerKwh, serviceFeeRate, minCharge, maxSingleCharge float64) error {
	if pricePerKwh <= 0 {
		return fmt.Errorf("电价必须大于0")
	}
	if serviceFeeRate < 0 || serviceFeeRate > 1 {
		return fmt.Errorf("服务费率必须在0-1之间")
	}
	if minCharge < 0 {
		return fmt.Errorf("最低消费不能为负")
	}
	if maxSingleCharge <= minCharge {
		return fmt.Errorf("最高限额必须大于最低消费")
	}

	p.PricePerKwh = pricePerKwh
	p.ServiceFeeRate = serviceFeeRate
	p.MinCharge = minCharge
	p.MaxSingleCharge = maxSingleCharge

	return nil
}
