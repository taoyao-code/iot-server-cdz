package bkv

import (
	"encoding/binary"
	"fmt"
	"time"
)

// ============ Week4: 刷卡充电数据结构 ============

// CardSwipeRequest 刷卡上报请求 (0x0B上行)
type CardSwipeRequest struct {
	CardNo   string // 卡号
	PhyID    string // 物理ID
	Balance  uint32 // 卡片余额（分）
	Reserved []byte // 预留字段
}

// ChargeCommand 充电指令 (0x0B下行)
type ChargeCommand struct {
	OrderNo     string // 订单号
	ChargeMode  uint8  // 充电模式：1=按时长,2=按电量,3=按功率,4=充满自停
	Amount      uint32 // 金额（分）
	Duration    uint32 // 时长（分钟）
	Power       uint16 // 功率（瓦）
	PricePerKwh uint32 // 电价（分/度）
	ServiceFee  uint16 // 服务费率（千分比，如50表示5%）
}

// OrderConfirmation 订单确认 (0x0F上行)
type OrderConfirmation struct {
	OrderNo string // 订单号
	Status  uint8  // 0=成功接受, 1=拒绝
	Reason  string // 拒绝原因
}

// OrderConfirmReply 订单确认回复 (0x0F下行)
type OrderConfirmReply struct {
	OrderNo string // 订单号
	Result  uint8  // 0=确认收到, 1=订单无效
}

// ChargeEndReport 充电结束上报 (0x0C上行)
// 格式1: 完整信息
type ChargeEndReport struct {
	OrderNo   string    // 订单号
	CardNo    string    // 卡号
	StartTime time.Time // 开始时间
	EndTime   time.Time // 结束时间
	Duration  uint32    // 时长（分钟）
	Energy    uint32    // 电量（Wh）
	Amount    uint32    // 金额（分）
	EndReason uint8     // 结束原因：0=正常,1=异常,2=手动停止
}

// ChargeEndReply 充电结束回复 (0x0C下行)
type ChargeEndReply struct {
	OrderNo string // 订单号
	Result  uint8  // 0=确认, 1=数据异常
}

// BalanceQuery 余额查询 (0x1A上行)
type BalanceQuery struct {
	CardNo string // 卡号
}

// BalanceResponse 余额响应 (0x1A下行)
type BalanceResponse struct {
	CardNo  string // 卡号
	Balance uint32 // 余额（分）
	Status  uint8  // 卡片状态：0=正常,1=无效,2=冻结
}

// ============ 协议编解码函数 ============

// ParseCardSwipeRequest 解析刷卡上报
func ParseCardSwipeRequest(data []byte) (*CardSwipeRequest, error) {
	if len(data) < 20 {
		return nil, fmt.Errorf("card swipe data too short: %d", len(data))
	}

	req := &CardSwipeRequest{}

	// 卡号（10字节BCD码）
	cardBytes := data[0:10]
	req.CardNo = bcdToString(cardBytes)

	// 物理ID（4字节）
	phyBytes := data[10:14]
	req.PhyID = fmt.Sprintf("%02X%02X%02X%02X", phyBytes[0], phyBytes[1], phyBytes[2], phyBytes[3])

	// 余额（4字节，大端序）
	req.Balance = binary.BigEndian.Uint32(data[14:18])

	// 预留字段
	if len(data) > 18 {
		req.Reserved = data[18:]
	}

	return req, nil
}

// EncodeChargeCommand 编码充电指令
func EncodeChargeCommand(cmd *ChargeCommand) []byte {
	buf := make([]byte, 0, 64)

	// 订单号（16字节ASCII）
	orderBytes := make([]byte, 16)
	copy(orderBytes, cmd.OrderNo)
	buf = append(buf, orderBytes...)

	// 充电模式（1字节）
	buf = append(buf, cmd.ChargeMode)

	// 金额（4字节，大端序）
	amountBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(amountBytes, cmd.Amount)
	buf = append(buf, amountBytes...)

	// 时长（4字节，大端序）
	durationBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(durationBytes, cmd.Duration)
	buf = append(buf, durationBytes...)

	// 功率（2字节，大端序）
	powerBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(powerBytes, cmd.Power)
	buf = append(buf, powerBytes...)

	// 电价（4字节，大端序）
	priceBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(priceBytes, cmd.PricePerKwh)
	buf = append(buf, priceBytes...)

	// 服务费率（2字节，大端序）
	feeBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(feeBytes, cmd.ServiceFee)
	buf = append(buf, feeBytes...)

	return buf
}

// ParseOrderConfirmation 解析订单确认
func ParseOrderConfirmation(data []byte) (*OrderConfirmation, error) {
	if len(data) < 17 {
		return nil, fmt.Errorf("order confirmation data too short: %d", len(data))
	}

	conf := &OrderConfirmation{}

	// 订单号（16字节ASCII）
	conf.OrderNo = string(data[0:16])

	// 状态（1字节）
	conf.Status = data[16]

	// 拒绝原因（剩余字节）
	if len(data) > 17 && conf.Status != 0 {
		conf.Reason = string(data[17:])
	}

	return conf, nil
}

// EncodeOrderConfirmReply 编码订单确认回复
func EncodeOrderConfirmReply(reply *OrderConfirmReply) []byte {
	buf := make([]byte, 17)

	// 订单号（16字节）
	copy(buf[0:16], reply.OrderNo)

	// 结果（1字节）
	buf[16] = reply.Result

	return buf
}

// ParseChargeEndReport 解析充电结束上报
func ParseChargeEndReport(data []byte) (*ChargeEndReport, error) {
	if len(data) < 41 {
		return nil, fmt.Errorf("charge end data too short: %d", len(data))
	}

	report := &ChargeEndReport{}

	// 订单号（16字节）
	report.OrderNo = string(data[0:16])

	// 卡号（10字节BCD）
	cardBytes := data[16:26]
	report.CardNo = bcdToString(cardBytes)

	// 开始时间（4字节Unix时间戳）
	startTs := binary.BigEndian.Uint32(data[26:30])
	report.StartTime = time.Unix(int64(startTs), 0)

	// 结束时间（4字节Unix时间戳）
	endTs := binary.BigEndian.Uint32(data[30:34])
	report.EndTime = time.Unix(int64(endTs), 0)

	// 时长（2字节，分钟）
	report.Duration = uint32(binary.BigEndian.Uint16(data[34:36]))

	// 电量（4字节，Wh）
	report.Energy = binary.BigEndian.Uint32(data[36:40])

	// 金额（4字节，分）
	report.Amount = binary.BigEndian.Uint32(data[40:44])

	// 结束原因（1字节）
	if len(data) > 44 {
		report.EndReason = data[44]
	}

	return report, nil
}

// EncodeChargeEndReply 编码充电结束回复
func EncodeChargeEndReply(reply *ChargeEndReply) []byte {
	buf := make([]byte, 17)

	// 订单号（16字节）
	copy(buf[0:16], reply.OrderNo)

	// 结果（1字节）
	buf[16] = reply.Result

	return buf
}

// ParseBalanceQuery 解析余额查询
func ParseBalanceQuery(data []byte) (*BalanceQuery, error) {
	if len(data) < 10 {
		return nil, fmt.Errorf("balance query data too short: %d", len(data))
	}

	query := &BalanceQuery{}

	// 卡号（10字节BCD）
	query.CardNo = bcdToString(data[0:10])

	return query, nil
}

// EncodeBalanceResponse 编码余额响应
func EncodeBalanceResponse(resp *BalanceResponse) []byte {
	buf := make([]byte, 15)

	// 卡号（10字节BCD）
	cardBytes := stringToBCD(resp.CardNo, 10)
	copy(buf[0:10], cardBytes)

	// 余额（4字节，大端序）
	binary.BigEndian.PutUint32(buf[10:14], resp.Balance)

	// 状态（1字节）
	buf[14] = resp.Status

	return buf
}

// ============ 辅助函数 ============

// bcdToString BCD码转字符串
func bcdToString(bcd []byte) string {
	result := ""
	for _, b := range bcd {
		high := (b >> 4) & 0x0F
		low := b & 0x0F
		result += fmt.Sprintf("%d%d", high, low)
	}
	return result
}

// stringToBCD 字符串转BCD码
func stringToBCD(s string, length int) []byte {
	result := make([]byte, length)
	// 补齐到偶数位
	if len(s)%2 != 0 {
		s = "0" + s
	}
	// 截取或补零
	if len(s) > length*2 {
		s = s[0 : length*2]
	} else {
		s = fmt.Sprintf("%0*s", length*2, s)
	}

	for i := 0; i < len(s); i += 2 {
		high := s[i] - '0'
		low := s[i+1] - '0'
		result[i/2] = (high << 4) | low
	}
	return result
}

// GenerateOrderNo 生成订单号
func GenerateOrderNo(cardNo, phyID string) string {
	// 格式：CARD + 时间戳(10位) + 卡号后6位
	ts := time.Now().Unix()
	suffix := cardNo
	if len(cardNo) > 6 {
		suffix = cardNo[len(cardNo)-6:]
	}
	return fmt.Sprintf("CARD%010d%s", ts, suffix)
}
