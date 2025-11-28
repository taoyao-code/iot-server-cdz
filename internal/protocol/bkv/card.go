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
// 协议文档 2.2.3 刷卡充电结束上报格式
type ChargeEndReport struct {
	SocketNo        uint8    // 插座号
	Version         uint16   // 插座软件版本
	Temperature     uint8    // 插座温度
	RSSI            uint8    // 信号强度
	Port            uint8    // 插孔号: 0=A孔, 1=B孔
	Status          uint8    // 插座状态位
	BusinessNo      uint16   // 业务号（与开始充电时对应）
	Power           uint16   // 瞬时功率
	Current         uint16   // 瞬时电流
	Energy          uint32   // 用电量（Wh）
	Duration        uint32   // 充电时间（分钟）
	CardNo          string   // 卡号（6字节）
	CardType        uint8    // 卡类型: 0=在线卡
	ChargeMode      uint8    // 计费模式: 1=按时, 2=按量, 3=按功率
	Amount          uint32   // 花费金额（分，仅按功率模式）
	SettlementPower uint16   // 结算功率（仅按功率模式）
	LevelCount      uint8    // 档位数（仅按功率模式）
	LevelDurations  []uint16 // 每档充电时间（仅按功率模式）

	// 兼容旧字段（用于上层业务逻辑）
	OrderNo   string    // 订单号（由业务号生成）
	EndReason uint8     // 结束原因（从状态位推导）
	StartTime time.Time // 协议未提供，留空
	EndTime   time.Time // 协议未提供，留空
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

// ParseChargeEndReport 解析刷卡充电结束上报 (0x0C上行)
// 协议文档 2.2.3 刷卡充电结束格式
//
// 输入格式 (f.Data):
//
//	[length:2][subCmd=0x0C][socketNo][version:2][temp][rssi][port][status]
//	[businessNo:2][power:2][current:2][energy:2][duration:2][cardNo:6][cardType]
//	[chargeMode][amount:2?][settlementPower:2?][levelCount?][levelDurations:2*n?]
//
// 最小长度: 2+1+25 = 28 字节
func ParseChargeEndReport(data []byte) (*ChargeEndReport, error) {
	// 最小长度检查: length(2) + subCmd(1) + payload(25)
	if len(data) < 28 {
		return nil, fmt.Errorf("charge end data too short: %d (need at least 28)", len(data))
	}

	// 解析帧长度和子命令
	declLen := int(binary.BigEndian.Uint16(data[0:2]))
	subCmd := data[2]
	if subCmd != 0x0C {
		return nil, fmt.Errorf("invalid sub_cmd for charge end: 0x%02X (expected 0x0C)", subCmd)
	}

	// payload 从 data[3] 开始
	payload := data[3:]
	expectedLen := declLen - 1 // 减去 subCmd 占用的 1 字节
	if len(payload) < expectedLen {
		// 使用可用长度继续解析
		if len(payload) < 25 {
			return nil, fmt.Errorf("charge end payload too short: %d (need at least 25)", len(payload))
		}
	}

	report := &ChargeEndReport{
		SocketNo:    payload[0],
		Version:     binary.BigEndian.Uint16(payload[1:3]),
		Temperature: payload[3],
		RSSI:        payload[4],
		Port:        payload[5],
		Status:      payload[6],
		BusinessNo:  binary.BigEndian.Uint16(payload[7:9]),
		Power:       binary.BigEndian.Uint16(payload[9:11]),
		Current:     binary.BigEndian.Uint16(payload[11:13]),
		Energy:      uint32(binary.BigEndian.Uint16(payload[13:15])), // Wh
		Duration:    uint32(binary.BigEndian.Uint16(payload[15:17])), // 分钟
	}

	// 卡号 (6字节)
	if len(payload) >= 23 {
		report.CardNo = bcdToString(payload[17:23])
	}

	// 卡类型
	if len(payload) >= 24 {
		report.CardType = payload[23]
	}

	// 计费模式
	if len(payload) >= 25 {
		report.ChargeMode = payload[24]
	}

	// 按功率模式的额外字段
	if report.ChargeMode == 3 {
		if len(payload) >= 27 {
			report.Amount = uint32(binary.BigEndian.Uint16(payload[25:27]))
		}
		if len(payload) >= 29 {
			report.SettlementPower = binary.BigEndian.Uint16(payload[27:29])
		}
		if len(payload) >= 30 {
			report.LevelCount = payload[29]
			// 解析每档充电时间
			pos := 30
			for i := uint8(0); i < report.LevelCount && i < 5; i++ {
				if pos+2 > len(payload) {
					break
				}
				levelDur := binary.BigEndian.Uint16(payload[pos : pos+2])
				report.LevelDurations = append(report.LevelDurations, levelDur)
				pos += 2
			}
		}
	}

	// 生成兼容字段
	report.OrderNo = fmt.Sprintf("%04X", report.BusinessNo)
	report.EndReason = uint8(deriveEndReasonFromStatus(report.Status))

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
