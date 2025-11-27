package bkv

// MapPort 将业务端口号映射为协议端口号（负数视为0）。
func MapPort(port int) int {
	if port < 0 {
		return 0
	}
	return port
}

// EncodeStartControlPayload 构造0x0015开始充电控制命令的载荷（不含长度前缀）。
// 布局: [0x07][插座1B][插孔1B][开关1B][模式1B][时长2B][电量2B]
// 协议文档2.2.8：平台下发控制命令不包含业务号，bytes[7-8]是充电电量而非业务号
// 设备会在ACK回复中生成并返回业务号，充电结束时使用相同的业务号
// energyWh: 按时充电时传0，按量充电时传电量(单位wh，0-65535)
func EncodeStartControlPayload(socketNo uint8, port uint8, mode uint8, durationMin uint16, energyWh uint16) []byte {
	buf := make([]byte, 9)
	buf[0] = 0x07                   // 子命令
	buf[1] = socketNo               // 插座号 (1-250)
	buf[2] = port                   // 插孔号 (0=A孔, 1=B孔)
	buf[3] = 0x01                   // 开关 (0x01=开, 0x00=关)
	buf[4] = mode                   // 模式 (0x01=按时, 0x00=按量)
	buf[5] = byte(durationMin >> 8) // 充电时长高字节
	buf[6] = byte(durationMin)      // 充电时长低字节
	buf[7] = byte(energyWh >> 8)    // 充电电量高字节
	buf[8] = byte(energyWh)         // 充电电量低字节
	return buf
}

// EncodeStopControlPayload 构造0x0015停止充电控制命令的载荷（不含长度前缀）。
func EncodeStopControlPayload(socketNo uint8, port uint8, businessNo uint16) []byte {
	buf := make([]byte, 9)
	buf[0] = 0x07
	buf[1] = socketNo
	buf[2] = port
	buf[3] = 0x00
	buf[4] = 0x01
	buf[5] = 0x00
	buf[6] = 0x00
	buf[7] = byte(businessNo >> 8)
	buf[8] = byte(businessNo)
	return buf
}

// EncodeQueryPortStatusPayload 构造0x0015的查询插座状态载荷。
func EncodeQueryPortStatusPayload(socketNo uint8) []byte {
	payload := make([]byte, 4)
	payload[0] = 0x00
	payload[1] = 0x02
	payload[2] = 0x1D
	payload[3] = socketNo
	return payload
}

// WrapControlPayload 为控制命令增加长度前缀，形成 0x0015 命令的 Data 部分。
// 长度字段包含命令字节后的参数长度（不含0x07）。
func WrapControlPayload(inner []byte) []byte {
	if len(inner) == 0 {
		return []byte{0x00, 0x00}
	}
	paramLen := len(inner) - 1
	payload := make([]byte, 2+len(inner))
	payload[0] = byte(paramLen >> 8)
	payload[1] = byte(paramLen)
	copy(payload[2:], inner)
	return payload
}
