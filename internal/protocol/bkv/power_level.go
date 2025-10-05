package bkv

import (
	"encoding/binary"
	"fmt"
)

// Week 8: 按功率分档充电协议（cmd=0x17/0x18）

// ===== 0x17 按功率分档充电命令 =====

// PowerLevelV2 功率档位（扩展版）
type PowerLevelV2 struct {
	PowerW     uint16 // 功率(W)
	PriceCents uint16 // 价格(分/度)
	Duration   uint16 // 时长(分钟)
}

// PowerLevelCommand 按功率分档充电命令（下行）
type PowerLevelCommand struct {
	PortNo     uint8          // 端口号
	LevelCount uint8          // 档位数量 (1-5)
	Levels     []PowerLevelV2 // 功率档位列表
}

// EncodePowerLevelCommand 编码按功率分档充电命令
func EncodePowerLevelCommand(cmd *PowerLevelCommand) []byte {
	// 基础长度：端口号(1) + 档位数(1) + 档位数据(levelCount * 6)
	levelCount := cmd.LevelCount
	if levelCount > 5 {
		levelCount = 5 // 最多5档
	}
	if levelCount > uint8(len(cmd.Levels)) {
		levelCount = uint8(len(cmd.Levels))
	}

	dataLen := 2 + int(levelCount)*6
	buf := make([]byte, dataLen)

	// 端口号 (1字节)
	buf[0] = cmd.PortNo

	// 档位数量 (1字节)
	buf[1] = levelCount

	// 各档位数据 (每档6字节：功率2 + 价格2 + 时长2)
	offset := 2
	for i := 0; i < int(levelCount); i++ {
		level := cmd.Levels[i]

		// 功率 (2字节，大端)
		binary.BigEndian.PutUint16(buf[offset:offset+2], level.PowerW)
		offset += 2

		// 价格 (2字节，大端，单位：分/度)
		binary.BigEndian.PutUint16(buf[offset:offset+2], level.PriceCents)
		offset += 2

		// 时长 (2字节，大端，单位：分钟)
		binary.BigEndian.PutUint16(buf[offset:offset+2], level.Duration)
		offset += 2
	}

	return buf
}

// ParsePowerLevelCommand 解析按功率分档充电命令（用于测试）
func ParsePowerLevelCommand(data []byte) (*PowerLevelCommand, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("power level command too short: %d", len(data))
	}

	cmd := &PowerLevelCommand{
		PortNo:     data[0],
		LevelCount: data[1],
	}

	expectedLen := 2 + int(cmd.LevelCount)*6
	if len(data) < expectedLen {
		return nil, fmt.Errorf("power level command incomplete: expected %d, got %d", expectedLen, len(data))
	}

	offset := 2
	for i := 0; i < int(cmd.LevelCount); i++ {
		level := PowerLevelV2{
			PowerW:     binary.BigEndian.Uint16(data[offset : offset+2]),
			PriceCents: binary.BigEndian.Uint16(data[offset+2 : offset+4]),
			Duration:   binary.BigEndian.Uint16(data[offset+4 : offset+6]),
		}
		cmd.Levels = append(cmd.Levels, level)
		offset += 6
	}

	return cmd, nil
}

// ===== 0x18 按功率充电结束上报 =====

// PowerLevelEndReport 按功率充电结束上报（上行）
type PowerLevelEndReport struct {
	PortNo        uint8             // 端口号
	TotalDuration uint16            // 总时长(分钟)
	TotalEnergy   uint32            // 总电量(0.01度)
	TotalAmount   uint32            // 总金额(分)
	EndReason     uint8             // 结束原因: 0=正常结束, 1=用户停止, 2=故障, 3=超时
	LevelUsage    []PowerLevelUsage // 各档位使用情况
}

// PowerLevelUsage 档位使用情况
type PowerLevelUsage struct {
	LevelNo  uint8  // 档位编号 (1-5)
	Duration uint16 // 使用时长(分钟)
	Energy   uint32 // 消耗电量(0.01度)
	Amount   uint32 // 消费金额(分)
}

// ParsePowerLevelEndReport 解析按功率充电结束上报
func ParsePowerLevelEndReport(data []byte) (*PowerLevelEndReport, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("power level end report too short: %d", len(data))
	}

	report := &PowerLevelEndReport{
		PortNo:        data[0],
		TotalDuration: binary.BigEndian.Uint16(data[1:3]),
		TotalEnergy:   binary.BigEndian.Uint32(data[3:7]),
		TotalAmount:   binary.BigEndian.Uint32(data[7:11]),
		EndReason:     data[11],
	}

	// 解析各档位使用情况（可选）
	if len(data) > 12 {
		offset := 12
		// 档位数量 (1字节)
		if offset < len(data) {
			levelCount := data[offset]
			offset++

			// 各档位数据 (每档11字节：档位号1 + 时长2 + 电量4 + 金额4)
			for i := 0; i < int(levelCount) && offset+11 <= len(data); i++ {
				usage := PowerLevelUsage{
					LevelNo:  data[offset],
					Duration: binary.BigEndian.Uint16(data[offset+1 : offset+3]),
					Energy:   binary.BigEndian.Uint32(data[offset+3 : offset+7]),
					Amount:   binary.BigEndian.Uint32(data[offset+7 : offset+11]),
				}
				report.LevelUsage = append(report.LevelUsage, usage)
				offset += 11
			}
		}
	}

	return report, nil
}

// EncodePowerLevelEndReply 编码按功率充电结束确认（下行）
func EncodePowerLevelEndReply(portNo uint8, result uint8) []byte {
	return []byte{portNo, result} // 结果: 0=确认成功, 1=失败
}

// ===== 辅助函数 =====

// GetPowerLevelEndReasonDescription 获取结束原因描述
func GetPowerLevelEndReasonDescription(reason uint8) string {
	switch reason {
	case 0:
		return "正常结束"
	case 1:
		return "用户停止"
	case 2:
		return "设备故障"
	case 3:
		return "超时停止"
	default:
		return fmt.Sprintf("未知原因(%d)", reason)
	}
}

// ValidatePowerLevels 验证功率档位参数
func ValidatePowerLevels(levels []PowerLevelV2) error {
	if len(levels) == 0 {
		return fmt.Errorf("no power levels provided")
	}
	if len(levels) > 5 {
		return fmt.Errorf("too many power levels: %d (max 5)", len(levels))
	}

	for i, level := range levels {
		if level.PowerW == 0 {
			return fmt.Errorf("level %d: power cannot be zero", i+1)
		}
		if level.PriceCents == 0 {
			return fmt.Errorf("level %d: price cannot be zero", i+1)
		}
		if level.Duration == 0 {
			return fmt.Errorf("level %d: duration cannot be zero", i+1)
		}
	}

	return nil
}
