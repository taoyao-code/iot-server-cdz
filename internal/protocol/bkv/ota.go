package bkv

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
)

// Week 7: OTA升级协议（cmd=0x07）

// ===== OTA升级命令 =====

// OTACommand OTA升级命令（下行）
type OTACommand struct {
	TargetType uint8  // 升级目标类型: 01=DTU主机, 02=插座
	SocketNo   uint8  // 插座编号 (仅当TargetType=02时有效)
	FTPServer  string // FTP服务器IP地址
	FTPPort    uint16 // FTP端口
	FileName   string // 文件名 (13字节ASCII hex)
}

// OTAResponse OTA升级响应（上行）
type OTAResponse struct {
	TargetType uint8  // 升级目标类型
	SocketNo   uint8  // 插座编号
	Result     uint8  // 结果: 0=成功开始升级, 1=失败, 2=FTP连接失败, 3=文件不存在
	Reason     string // 失败原因
}

// OTAProgress OTA升级进度上报（上行）
type OTAProgress struct {
	TargetType uint8  // 升级目标类型
	SocketNo   uint8  // 插座编号
	Progress   uint8  // 升级进度 0-100
	Status     uint8  // 状态: 0=下载中, 1=安装中, 2=完成, 3=失败
	ErrorMsg   string // 错误信息
}

// EncodeOTACommand 编码OTA升级命令
func EncodeOTACommand(cmd *OTACommand) []byte {
	buf := make([]byte, 20)

	// 升级目标类型 (1字节)
	buf[0] = cmd.TargetType

	// 插座编号 (1字节，仅当TargetType=02时有效)
	if cmd.TargetType == 0x02 {
		buf[1] = cmd.SocketNo
	} else {
		buf[1] = 0x00
	}

	// FTP服务器IP (4字节)
	ip := net.ParseIP(cmd.FTPServer)
	if ip == nil {
		// 如果解析失败，使用0.0.0.0
		copy(buf[2:6], []byte{0, 0, 0, 0})
	} else {
		ip4 := ip.To4()
		if ip4 != nil {
			copy(buf[2:6], ip4)
		}
	}

	// FTP端口 (2字节，大端)
	binary.BigEndian.PutUint16(buf[6:8], cmd.FTPPort)

	// 文件名 (12字节，ASCII)
	// 如果文件名不足12字节，用空格填充
	fileName := cmd.FileName
	if len(fileName) > 12 {
		fileName = fileName[:12]
	}
	copy(buf[8:20], []byte(fileName))
	// 填充空格
	for i := 8 + len(fileName); i < 20; i++ {
		buf[i] = 0x20 // 空格
	}

	return buf
}

// ParseOTAResponse 解析OTA升级响应
func ParseOTAResponse(data []byte) (*OTAResponse, error) {
	if len(data) < 3 {
		return nil, fmt.Errorf("OTA response too short: %d", len(data))
	}

	resp := &OTAResponse{
		TargetType: data[0],
		SocketNo:   data[1],
		Result:     data[2],
	}

	// 如果有失败原因
	if len(data) > 3 && resp.Result != 0 {
		resp.Reason = string(data[3:])
	}

	return resp, nil
}

// ParseOTAProgress 解析OTA升级进度上报
func ParseOTAProgress(data []byte) (*OTAProgress, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("OTA progress too short: %d", len(data))
	}

	progress := &OTAProgress{
		TargetType: data[0],
		SocketNo:   data[1],
		Progress:   data[2],
		Status:     data[3],
	}

	// 如果有错误信息
	if len(data) > 4 && progress.Status == 3 {
		progress.ErrorMsg = string(data[4:])
	}

	return progress, nil
}

// ===== 辅助函数 =====

// ipStringToBytes 将IP地址字符串转为4字节
func ipStringToBytes(ipStr string) ([4]byte, error) {
	var bytes [4]byte
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return bytes, fmt.Errorf("invalid IP address: %s", ipStr)
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return bytes, fmt.Errorf("not an IPv4 address: %s", ipStr)
	}
	copy(bytes[:], ip4)
	return bytes, nil
}

// ipBytesToString 将4字节转为IP地址字符串
func ipBytesToString(bytes []byte) string {
	if len(bytes) < 4 {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", bytes[0], bytes[1], bytes[2], bytes[3])
}

// fileNameToBytes 将文件名转为12字节（空格填充）
func fileNameToBytes(name string) [12]byte {
	var bytes [12]byte
	// 填充空格
	for i := 0; i < 12; i++ {
		bytes[i] = 0x20
	}
	// 复制文件名
	n := len(name)
	if n > 12 {
		n = 12
	}
	copy(bytes[:n], []byte(name))
	return bytes
}

// fileNameFromBytes 从12字节中提取文件名（去除空格）
func fileNameFromBytes(bytes []byte) string {
	if len(bytes) < 12 {
		return ""
	}
	return strings.TrimSpace(string(bytes[:12]))
}

// GetOTAResultDescription 获取OTA结果描述
func GetOTAResultDescription(result uint8) string {
	switch result {
	case 0:
		return "成功开始升级"
	case 1:
		return "升级失败"
	case 2:
		return "FTP连接失败"
	case 3:
		return "文件不存在"
	default:
		return fmt.Sprintf("未知结果(%d)", result)
	}
}

// GetOTAStatusDescription 获取OTA状态描述
func GetOTAStatusDescription(status uint8) string {
	switch status {
	case 0:
		return "下载中"
	case 1:
		return "安装中"
	case 2:
		return "完成"
	case 3:
		return "失败"
	default:
		return fmt.Sprintf("未知状态(%d)", status)
	}
}
