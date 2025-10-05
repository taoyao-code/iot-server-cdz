package bkv

import (
	"fmt"
)

// Week 6: 组网管理协议（cmd=0x08/0x09/0x0A）

// ===== 0x08 刷新插座列表 =====

// NetworkRefreshCommand 刷新插座列表命令（下行）
type NetworkRefreshCommand struct {
	// 无参数，仅刷新
}

// NetworkRefreshResponse 刷新插座列表响应（上行）
type NetworkRefreshResponse struct {
	SocketCount uint8        // 插座数量
	Sockets     []SocketInfo // 插座信息列表
}

// SocketInfo 插座信息
type SocketInfo struct {
	SocketNo       uint8  // 插座编号 (1-250)
	SocketMAC      string // 插座MAC地址 (6字节)
	SocketUID      string // 插座UID (4字节)
	Channel        uint8  // 信道 (1-15)
	SignalStrength int8   // 信号强度 (RSSI)
	Status         uint8  // 状态: 0=离线, 1=在线
}

// EncodeNetworkRefreshCommand 编码刷新列表命令
func EncodeNetworkRefreshCommand() []byte {
	// 无参数，返回空数据
	return []byte{}
}

// ParseNetworkRefreshResponse 解析刷新列表响应
func ParseNetworkRefreshResponse(data []byte) (*NetworkRefreshResponse, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("refresh response too short: %d", len(data))
	}

	resp := &NetworkRefreshResponse{}
	resp.SocketCount = data[0]

	// 每个插座信息占 14 字节
	expectedLen := 1 + int(resp.SocketCount)*14
	if len(data) < expectedLen {
		return nil, fmt.Errorf("refresh response incomplete: expected %d, got %d", expectedLen, len(data))
	}

	offset := 1
	for i := 0; i < int(resp.SocketCount); i++ {
		socket := SocketInfo{}

		// 插座编号 (1字节)
		socket.SocketNo = data[offset]
		offset++

		// MAC地址 (6字节)
		socket.SocketMAC = fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
			data[offset], data[offset+1], data[offset+2],
			data[offset+3], data[offset+4], data[offset+5])
		offset += 6

		// UID (4字节)
		socket.SocketUID = fmt.Sprintf("%02X%02X%02X%02X",
			data[offset], data[offset+1], data[offset+2], data[offset+3])
		offset += 4

		// 信道 (1字节)
		socket.Channel = data[offset]
		offset++

		// 信号强度 (1字节，有符号)
		socket.SignalStrength = int8(data[offset])
		offset++

		// 状态 (1字节)
		socket.Status = data[offset]
		offset++

		resp.Sockets = append(resp.Sockets, socket)
	}

	return resp, nil
}

// ===== 0x09 添加插座 =====

// NetworkAddNodeCommand 添加插座命令（下行）
type NetworkAddNodeCommand struct {
	SocketNo  uint8  // 插座编号 (1-250)
	SocketMAC string // 插座MAC地址
	Channel   uint8  // 信道 (1-15)
}

// NetworkAddNodeResponse 添加插座响应（上行）
type NetworkAddNodeResponse struct {
	SocketNo uint8  // 插座编号
	Result   uint8  // 0=成功, 1=失败, 2=编号冲突, 3=信道无效
	Reason   string // 失败原因
}

// EncodeNetworkAddNodeCommand 编码添加插座命令
func EncodeNetworkAddNodeCommand(cmd *NetworkAddNodeCommand) []byte {
	buf := make([]byte, 8)

	// 插座编号 (1字节)
	buf[0] = cmd.SocketNo

	// MAC地址 (6字节)
	// 解析 "AA:BB:CC:DD:EE:FF" 格式
	var mac [6]byte
	fmt.Sscanf(cmd.SocketMAC, "%02X:%02X:%02X:%02X:%02X:%02X",
		&mac[0], &mac[1], &mac[2], &mac[3], &mac[4], &mac[5])
	copy(buf[1:7], mac[:])

	// 信道 (1字节)
	buf[7] = cmd.Channel

	return buf
}

// ParseNetworkAddNodeResponse 解析添加插座响应
func ParseNetworkAddNodeResponse(data []byte) (*NetworkAddNodeResponse, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("add node response too short: %d", len(data))
	}

	resp := &NetworkAddNodeResponse{}
	resp.SocketNo = data[0]
	resp.Result = data[1]

	// 如果有失败原因
	if len(data) > 2 && resp.Result != 0 {
		resp.Reason = string(data[2:])
	}

	return resp, nil
}

// ===== 0x0A 删除插座 =====

// NetworkDeleteNodeCommand 删除插座命令（下行）
type NetworkDeleteNodeCommand struct {
	SocketNo uint8 // 插座编号 (1-250)
}

// NetworkDeleteNodeResponse 删除插座响应（上行）
type NetworkDeleteNodeResponse struct {
	SocketNo uint8  // 插座编号
	Result   uint8  // 0=成功, 1=失败, 2=插座不存在
	Reason   string // 失败原因
}

// EncodeNetworkDeleteNodeCommand 编码删除插座命令
func EncodeNetworkDeleteNodeCommand(cmd *NetworkDeleteNodeCommand) []byte {
	return []byte{cmd.SocketNo}
}

// ParseNetworkDeleteNodeResponse 解析删除插座响应
func ParseNetworkDeleteNodeResponse(data []byte) (*NetworkDeleteNodeResponse, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("delete node response too short: %d", len(data))
	}

	resp := &NetworkDeleteNodeResponse{}
	resp.SocketNo = data[0]
	resp.Result = data[1]

	// 如果有失败原因
	if len(data) > 2 && resp.Result != 0 {
		resp.Reason = string(data[2:])
	}

	return resp, nil
}

// ===== 辅助函数 =====

// macStringToBytes 将MAC地址字符串转为字节数组
func macStringToBytes(mac string) ([6]byte, error) {
	var bytes [6]byte
	n, err := fmt.Sscanf(mac, "%02X:%02X:%02X:%02X:%02X:%02X",
		&bytes[0], &bytes[1], &bytes[2], &bytes[3], &bytes[4], &bytes[5])
	if err != nil || n != 6 {
		return bytes, fmt.Errorf("invalid MAC format: %s", mac)
	}
	return bytes, nil
}

// macBytesToString 将MAC字节数组转为字符串
func macBytesToString(bytes []byte) string {
	if len(bytes) < 6 {
		return ""
	}
	return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
		bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5])
}
