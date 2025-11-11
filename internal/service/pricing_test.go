package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewPricingEngine 测试创建计费引擎
func TestNewPricingEngine(t *testing.T) {
	engine := NewPricingEngine()

	assert.NotNil(t, engine)
	assert.Equal(t, 0.5, engine.PricePerKwh, "默认电价应为0.5元/度")
	assert.Equal(t, 0.05, engine.ServiceFeeRate, "默认服务费率应为5%")
	assert.Equal(t, 1.0, engine.MinCharge, "默认最低消费应为1元")
	assert.Equal(t, 100.0, engine.MaxSingleCharge, "默认单次最高应为100元")
}

// TestCalculateByDuration 测试按时长计费
func TestCalculateByDuration(t *testing.T) {
	engine := NewPricingEngine()

	// 场景1: 正常时长充电（60分钟，功率1000W）
	params := engine.CalculateByDuration(60, 100.0, 1000)
	assert.Equal(t, 1, params.Mode)
	assert.Greater(t, params.Duration, uint32(0))
	assert.Greater(t, params.Energy, 0.0)
	assert.Equal(t, uint16(1000), params.Power)
	assert.GreaterOrEqual(t, params.Amount, engine.MinCharge)
	t.Logf("60分钟/1000W: 金额%.2f元, 预计%d分钟, %.2f度",
		params.Amount, params.Duration, params.Energy)
}

// TestCalculateByDuration_MinCharge 测试最低消费限制
func TestCalculateByDuration_MinCharge(t *testing.T) {
	engine := NewPricingEngine()

	// 极短时长（1分钟）应被最低消费限制
	params := engine.CalculateByDuration(1, 100.0, 100)
	assert.Equal(t, engine.MinCharge, params.Amount, "应用最低消费限制")
}

// TestCalculateByDuration_MaxAmount 测试最大金额限制
func TestCalculateByDuration_MaxAmount(t *testing.T) {
	engine := NewPricingEngine()

	// 大功率长时间，但限制最大金额为5元
	params := engine.CalculateByDuration(120, 5.0, 10000)
	assert.LessOrEqual(t, params.Amount, 5.0, "不应超过最大金额限制")
	assert.GreaterOrEqual(t, params.Amount, engine.MinCharge, "不应低于最低消费")
}

// TestCalculateByEnergy 测试按电量计费
func TestCalculateByEnergy(t *testing.T) {
	engine := NewPricingEngine()

	// 场景1: 正常电量（5度）
	params := engine.CalculateByEnergy(5.0, 100.0)
	assert.Equal(t, 2, params.Mode)
	assert.Equal(t, 5.0, params.Energy)

	// 计算期望金额：5度 * 0.5元/度 * (1 + 0.05) = 2.625元
	expectedAmount := 5.0 * 0.5 * 1.05
	assert.InDelta(t, expectedAmount, params.Amount, 0.01)
	t.Logf("5度: 金额%.2f元", params.Amount)
}

// TestCalculateByEnergy_MinCharge 测试最低消费限制
func TestCalculateByEnergy_MinCharge(t *testing.T) {
	engine := NewPricingEngine()

	// 极小电量（0.1度），应被最低消费限制
	params := engine.CalculateByEnergy(0.1, 100.0)
	assert.Equal(t, engine.MinCharge, params.Amount)
	// 当达到最低消费时，反推的实际电量会更大（1元/0.5元/度/1.05√1.9度）
	assert.Greater(t, params.Energy, 0.1, "反推的电量会达到最低消费所对应的电量")
}

// TestCalculateByEnergy_MaxAmount 测试最大金额限制
func TestCalculateByEnergy_MaxAmount(t *testing.T) {
	engine := NewPricingEngine()

	// 大电量但限制最大金额
	params := engine.CalculateByEnergy(100.0, 10.0)
	assert.LessOrEqual(t, params.Amount, 10.0)
	assert.Less(t, params.Energy, 100.0, "实际电量会被反推调整")
}

// TestCalculateByPower 测试按功率计费
func TestCalculateByPower(t *testing.T) {
	engine := NewPricingEngine()

	// 场景: 2000W功率，充电30分钟
	params := engine.CalculateByPower(2000, 30, 100.0)
	assert.Equal(t, 3, params.Mode)
	assert.Equal(t, uint16(2000), params.Power)
	assert.Equal(t, uint32(30), params.Duration)

	// 预计电量：2000W * 0.5h / 1000 = 1度
	expectedEnergy := 2000.0 * 0.5 / 1000.0
	assert.InDelta(t, expectedEnergy, params.Energy, 0.01)
	t.Logf("2000W/30分钟: 金额%.2f元, %.2f度", params.Amount, params.Energy)
}

// TestCalculateFull 测试充满自停模式
func TestCalculateFull(t *testing.T) {
	engine := NewPricingEngine()

	// 场景1: 正常金额
	params := engine.CalculateFull(50.0)
	assert.Equal(t, 4, params.Mode)
	assert.Equal(t, 50.0, params.Amount)

	// 场景2: 超过最高限额
	params2 := engine.CalculateFull(200.0)
	assert.Equal(t, engine.MaxSingleCharge, params2.Amount, "应限制在最高限额")

	// 场景3: 低于最低消费
	params3 := engine.CalculateFull(0.5)
	assert.Equal(t, engine.MinCharge, params3.Amount, "应提升到最低消费")
}

// TestCalculateActualCost 测试实际费用计算
func TestCalculateActualCost(t *testing.T) {
	engine := NewPricingEngine()

	// 场景1: 充了5000Wh (5度)
	cost := engine.CalculateActualCost(5000)
	expectedCost := 5.0 * 0.5 * 1.05 // 5度 * 0.5元/度 * 1.05
	assert.InDelta(t, expectedCost, cost, 0.01)
	t.Logf("5000Wh实际费用: %.2f元", cost)

	// 场景2: 极小电量
	cost2 := engine.CalculateActualCost(100) // 0.1度
	assert.Equal(t, engine.MinCharge, cost2, "应用最低消费")
}

// TestEstimateDuration 测试时长估算
func TestEstimateDuration(t *testing.T) {
	engine := NewPricingEngine()

	// 10元，1000W功率
	duration := engine.EstimateDuration(10.0, 1000)
	assert.Greater(t, duration, 0, "应返回正数时长")
	t.Logf("10元/1000W可充: %d分钟", duration)

	// 验证逻辑：10元 / 1.05 / 0.5 = 19.05度
	// 19.05度 * 1000W/h = 19.05小时 = 1143分钟
	expectedDuration := 1143 // 10 / 1.05 / 0.5 * 60
	assert.InDelta(t, float64(expectedDuration), float64(duration), 5.0, "时长估算应接近理论值")
}

// TestEstimateEnergy 测试电量估算
func TestEstimateEnergy(t *testing.T) {
	engine := NewPricingEngine()

	// 10元可充多少度
	energy := engine.EstimateEnergy(10.0)
	assert.Greater(t, energy, 0.0)

	// 验证：10元 / 1.05 / 0.5 = 19.05度
	expectedEnergy := 10.0 / 1.05 / 0.5
	assert.InDelta(t, expectedEnergy, energy, 0.1)
	t.Logf("10元可充: %.2f度", energy)
}

// TestValidateAmount 测试金额验证
func TestValidateAmount(t *testing.T) {
	engine := NewPricingEngine()

	// 正常金额
	err := engine.ValidateAmount(10.0)
	assert.NoError(t, err)

	// 低于最低消费
	err = engine.ValidateAmount(0.5)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "最低消费")

	// 超过最高限额
	err = engine.ValidateAmount(150.0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "最高限额")

	// 边界值：最低消费
	err = engine.ValidateAmount(engine.MinCharge)
	assert.NoError(t, err)

	// 边界值：最高限额
	err = engine.ValidateAmount(engine.MaxSingleCharge)
	assert.NoError(t, err)
}

// TestGetPriceInfo 测试获取计费信息
func TestGetPriceInfo(t *testing.T) {
	engine := NewPricingEngine()

	info := engine.GetPriceInfo()
	assert.NotNil(t, info)
	assert.Equal(t, 0.5, info["price_per_kwh"])
	assert.Equal(t, 0.05, info["service_fee_rate"])
	assert.Equal(t, 5.0, info["service_fee_pct"]) // 5%转换为百分比
	assert.Equal(t, 1.0, info["min_charge"])
	assert.Equal(t, 100.0, info["max_single_charge"])
}

// TestSetPricing 测试设置计费参数
func TestSetPricing(t *testing.T) {
	engine := NewPricingEngine()

	// 正常设置
	err := engine.SetPricing(0.6, 0.08, 2.0, 200.0)
	require.NoError(t, err)
	assert.Equal(t, 0.6, engine.PricePerKwh)
	assert.Equal(t, 0.08, engine.ServiceFeeRate)
	assert.Equal(t, 2.0, engine.MinCharge)
	assert.Equal(t, 200.0, engine.MaxSingleCharge)
}

// TestSetPricing_InvalidPricePerKwh 测试非法电价
func TestSetPricing_InvalidPricePerKwh(t *testing.T) {
	engine := NewPricingEngine()

	// 零电价
	err := engine.SetPricing(0, 0.05, 1.0, 100.0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "电价必须大于0")

	// 负电价
	err = engine.SetPricing(-0.5, 0.05, 1.0, 100.0)
	assert.Error(t, err)
}

// TestSetPricing_InvalidServiceFeeRate 测试非法服务费率
func TestSetPricing_InvalidServiceFeeRate(t *testing.T) {
	engine := NewPricingEngine()

	// 服务费率<0
	err := engine.SetPricing(0.5, -0.1, 1.0, 100.0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "服务费率必须在0-1之间")

	// 服务费率>1
	err = engine.SetPricing(0.5, 1.5, 1.0, 100.0)
	assert.Error(t, err)
}

// TestSetPricing_InvalidMinCharge 测试非法最低消费
func TestSetPricing_InvalidMinCharge(t *testing.T) {
	engine := NewPricingEngine()

	// 负最低消费
	err := engine.SetPricing(0.5, 0.05, -1.0, 100.0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "最低消费不能为负")
}

// TestSetPricing_InvalidMaxSingleCharge 测试非法最高限额
func TestSetPricing_InvalidMaxSingleCharge(t *testing.T) {
	engine := NewPricingEngine()

	// 最高限额 <= 最低消费
	err := engine.SetPricing(0.5, 0.05, 10.0, 5.0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "最高限额必须大于最低消费")

	// 相等
	err = engine.SetPricing(0.5, 0.05, 10.0, 10.0)
	assert.Error(t, err)
}

// TestPricing_RealWorldScenarios 测试真实世界场景
func TestPricing_RealWorldScenarios(t *testing.T) {
	engine := NewPricingEngine()

	scenarios := []struct {
		name     string
		mode     string
		testFunc func(*testing.T)
	}{
		{
			name: "家用充电桩-按时长",
			mode: "按时长",
			testFunc: func(t *testing.T) {
				// 7kW充电桩，充电2小时，最多消费20元
				params := engine.CalculateByDuration(120, 20.0, 7000)
				t.Logf("7kW充电桩/2小时: %.2f元, %.2f度", params.Amount, params.Energy)
				assert.GreaterOrEqual(t, params.Amount, engine.MinCharge)
				assert.LessOrEqual(t, params.Amount, 20.0)
			},
		},
		{
			name: "公共充电-按电量",
			mode: "按电量",
			testFunc: func(t *testing.T) {
				// 充10度电
				params := engine.CalculateByEnergy(10.0, 50.0)
				expectedCost := 10.0 * 0.5 * 1.05 // 5.25元
				t.Logf("充10度: %.2f元", params.Amount)
				assert.InDelta(t, expectedCost, params.Amount, 0.01)
			},
		},
		{
			name: "小额快充",
			mode: "按电量",
			testFunc: func(t *testing.T) {
				// 5元快充
				energy := engine.EstimateEnergy(5.0)
				t.Logf("5元可充: %.2f度", energy)
				assert.Greater(t, energy, 0.0)
			},
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			sc.testFunc(t)
		})
	}
}

// TestPricing_EdgeCases 测试边界情况
func TestPricing_EdgeCases(t *testing.T) {
	engine := NewPricingEngine()

	t.Run("零功率", func(t *testing.T) {
		params := engine.CalculateByDuration(60, 100.0, 0)
		// 零功率应该得到最低消费
		assert.Equal(t, engine.MinCharge, params.Amount)
	})

	t.Run("零时长", func(t *testing.T) {
		params := engine.CalculateByDuration(0, 100.0, 1000)
		// 零时长应该得到最低消费
		assert.Equal(t, engine.MinCharge, params.Amount)
	})

	t.Run("零电量", func(t *testing.T) {
		params := engine.CalculateByEnergy(0, 100.0)
		// 零电量应该得到最低消费
		assert.Equal(t, engine.MinCharge, params.Amount)
	})

	t.Run("极大功率", func(t *testing.T) {
		params := engine.CalculateByPower(50000, 60, 100.0) // 50kW
		// 应受最大金额限制
		assert.LessOrEqual(t, params.Amount, 100.0)
	})
}
