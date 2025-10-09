package bkv

import (
	"encoding/binary"
	"fmt"
)

// Week 10: 语音播报时间配置命令（cmd=0x1B）

// VoiceConfigCommand 语音配置命令（下行）
type VoiceConfigCommand struct {
	PeriodCount uint8         // 静音时段数量（最多2个）
	Periods     []VoicePeriod // 静音时段列表
}

// VoicePeriod 静音时段
type VoicePeriod struct {
	StartHour   uint8 // 开始小时 (0-23)
	StartMinute uint8 // 开始分钟 (0-59)
	EndHour     uint8 // 结束小时 (0-23)
	EndMinute   uint8 // 结束分钟 (0-59)
}

// EncodeVoiceConfigCommand 编码语音配置命令
func EncodeVoiceConfigCommand(cmd *VoiceConfigCommand) []byte {
	count := cmd.PeriodCount
	if count > 2 {
		count = 2 // 最多2个时段
	}

	buf := make([]byte, 1+int(count)*4)
	buf[0] = count

	offset := 1
	for i := 0; i < int(count); i++ {
		buf[offset] = cmd.Periods[i].StartHour
		buf[offset+1] = cmd.Periods[i].StartMinute
		buf[offset+2] = cmd.Periods[i].EndHour
		buf[offset+3] = cmd.Periods[i].EndMinute
		offset += 4
	}

	return buf
}

// VoiceConfigResponse 语音配置响应（上行）
type VoiceConfigResponse struct {
	Result  uint8  // 结果: 0=成功, 1=失败
	Message string // 消息
}

// ParseVoiceConfigResponse 解析语音配置响应
func ParseVoiceConfigResponse(data []byte) (*VoiceConfigResponse, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("voice config response too short")
	}

	resp := &VoiceConfigResponse{
		Result: data[0],
	}

	if len(data) > 1 {
		resp.Message = string(data[1:])
	}

	return resp, nil
}

// ValidateVoicePeriod 验证静音时段
func ValidateVoicePeriod(period *VoicePeriod) error {
	if period.StartHour > 23 {
		return fmt.Errorf("invalid start_hour: %d", period.StartHour)
	}
	if period.StartMinute > 59 {
		return fmt.Errorf("invalid start_minute: %d", period.StartMinute)
	}
	if period.EndHour > 23 {
		return fmt.Errorf("invalid end_hour: %d", period.EndHour)
	}
	if period.EndMinute > 59 {
		return fmt.Errorf("invalid end_minute: %d", period.EndMinute)
	}

	startMin := int(period.StartHour)*60 + int(period.StartMinute)
	endMin := int(period.EndHour)*60 + int(period.EndMinute)

	if startMin >= endMin {
		return fmt.Errorf("start time must be before end time")
	}

	return nil
}

// ===== 查询插座状态命令（cmd=0x1D/0x0D/0x0E） =====

// QuerySocketCommand 查询插座状态命令（下行）
type QuerySocketCommand struct {
	SocketNo uint8 // 插座编号 (0=查询所有)
}

// EncodeQuerySocketCommand 编码查询插座命令
func EncodeQuerySocketCommand(cmd *QuerySocketCommand) []byte {
	return []byte{cmd.SocketNo}
}

// SocketStateResponse 插座状态响应（上行）
type SocketStateResponse struct {
	SocketNo    uint8  // 插座编号
	Status      uint8  // 状态: 0=空闲, 1=充电中, 2=故障
	Voltage     uint16 // 电压 (0.1V)
	Current     uint16 // 电流 (mA)
	Power       uint16 // 功率 (W)
	Energy      uint32 // 已充电量 (0.01度)
	Duration    uint16 // 已充时长 (分钟)
	Temperature uint8  // 温度 (℃)
}

// ParseSocketStateResponse 解析插座状态响应
func ParseSocketStateResponse(data []byte) (*SocketStateResponse, error) {
	if len(data) < 14 {
		return nil, fmt.Errorf("socket state response too short: %d bytes", len(data))
	}

	resp := &SocketStateResponse{
		SocketNo:    data[0],
		Status:      data[1],
		Voltage:     binary.BigEndian.Uint16(data[2:4]),
		Current:     binary.BigEndian.Uint16(data[4:6]),
		Power:       binary.BigEndian.Uint16(data[6:8]),
		Energy:      binary.BigEndian.Uint32(data[8:12]),
		Duration:    binary.BigEndian.Uint16(data[12:14]),
		Temperature: 0,
	}

	if len(data) >= 15 {
		resp.Temperature = data[14]
	}

	return resp, nil
}

// GetSocketStatusDescription 获取插座状态描述
func GetSocketStatusDescription(status uint8) string {
	switch status {
	case 0:
		return "空闲"
	case 1:
		return "充电中"
	case 2:
		return "故障"
	default:
		return fmt.Sprintf("未知状态(%d)", status)
	}
}

// ===== 服务费充电命令（cmd=0x19） =====

// ServiceFeeCommand 服务费充电命令（下行）
type ServiceFeeCommand struct {
	PortNo      uint8  // 端口号
	Mode        uint8  // 充电模式: 0=按时, 1=按电量
	Duration    uint16 // 充电时长（分钟）或电量（0.01度）
	ElectricFee uint16 // 电费（分/度）
	ServiceFee  uint16 // 服务费（分/度）
}

// EncodeServiceFeeCommand 编码服务费充电命令
func EncodeServiceFeeCommand(cmd *ServiceFeeCommand) []byte {
	buf := make([]byte, 9)
	buf[0] = cmd.PortNo
	buf[1] = cmd.Mode
	binary.BigEndian.PutUint16(buf[2:4], cmd.Duration)
	binary.BigEndian.PutUint16(buf[4:6], cmd.ElectricFee)
	binary.BigEndian.PutUint16(buf[6:8], cmd.ServiceFee)
	buf[8] = 0 // 保留字段
	return buf
}

// ServiceFeeEndReport 服务费充电结束上报（上行）
type ServiceFeeEndReport struct {
	PortNo        uint8  // 端口号
	TotalDuration uint16 // 总时长（分钟）
	TotalEnergy   uint32 // 总电量（0.01度）
	ElectricFee   uint32 // 电费（分）
	ServiceFee    uint32 // 服务费（分）
	TotalAmount   uint32 // 总金额（分）
	EndReason     uint8  // 结束原因: 0=正常, 1=用户停止, 2=故障
}

// ParseServiceFeeEndReport 解析服务费充电结束上报
func ParseServiceFeeEndReport(data []byte) (*ServiceFeeEndReport, error) {
	if len(data) < 20 {
		return nil, fmt.Errorf("service fee end report too short: %d bytes", len(data))
	}

	report := &ServiceFeeEndReport{
		PortNo:        data[0],
		TotalDuration: binary.BigEndian.Uint16(data[1:3]),
		TotalEnergy:   binary.BigEndian.Uint32(data[3:7]),
		ElectricFee:   binary.BigEndian.Uint32(data[7:11]),
		ServiceFee:    binary.BigEndian.Uint32(data[11:15]),
		TotalAmount:   binary.BigEndian.Uint32(data[15:19]),
		EndReason:     data[19],
	}

	return report, nil
}

// EncodeServiceFeeEndReply 编码服务费充电结束回复
func EncodeServiceFeeEndReply(portNo uint8, result uint8) []byte {
	return []byte{portNo, result}
}
