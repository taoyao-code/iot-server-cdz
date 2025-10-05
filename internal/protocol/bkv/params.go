package bkv

import (
	"encoding/binary"
	"fmt"
)

// Week 9: 参数批量读写命令（cmd=0x01/0x02/0x03/0x04）

// ===== 0x01 批量读取参数 =====

// ParamReadRequest 批量读取参数请求（下行）
type ParamReadRequest struct {
	ParamIDs []uint16 // 参数ID列表（最多20个）
}

// EncodeParamReadRequest 编码参数读取请求
func EncodeParamReadRequest(req *ParamReadRequest) []byte {
	count := len(req.ParamIDs)
	if count > 20 {
		count = 20 // 最多20个
	}

	buf := make([]byte, 1+count*2)
	buf[0] = uint8(count) // 参数数量

	offset := 1
	for i := 0; i < count; i++ {
		binary.BigEndian.PutUint16(buf[offset:offset+2], req.ParamIDs[i])
		offset += 2
	}

	return buf
}

// ParamReadResponse 批量读取参数响应（上行）
type ParamReadResponse struct {
	Params []ParamValue // 参数值列表
}

// ParamValue 参数值
type ParamValue struct {
	ParamID uint16 // 参数ID
	Length  uint8  // 数据长度
	Value   []byte // 参数值
}

// ParseParamReadResponse 解析参数读取响应
func ParseParamReadResponse(data []byte) (*ParamReadResponse, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("param read response too short")
	}

	resp := &ParamReadResponse{}
	count := data[0]
	offset := 1

	for i := 0; i < int(count) && offset+3 <= len(data); i++ {
		param := ParamValue{
			ParamID: binary.BigEndian.Uint16(data[offset : offset+2]),
			Length:  data[offset+2],
		}
		offset += 3

		if offset+int(param.Length) > len(data) {
			return nil, fmt.Errorf("param %d: insufficient data", i)
		}

		param.Value = make([]byte, param.Length)
		copy(param.Value, data[offset:offset+int(param.Length)])
		offset += int(param.Length)

		resp.Params = append(resp.Params, param)
	}

	return resp, nil
}

// ===== 0x02 批量写入参数 =====

// ParamWriteRequest 批量写入参数请求（下行）
type ParamWriteRequest struct {
	Params []ParamValue // 参数值列表（最多10个）
}

// EncodeParamWriteRequest 编码参数写入请求
func EncodeParamWriteRequest(req *ParamWriteRequest) []byte {
	count := len(req.Params)
	if count > 10 {
		count = 10 // 最多10个
	}

	// 计算总长度
	totalLen := 1 // 数量字节
	for i := 0; i < count; i++ {
		totalLen += 3 + len(req.Params[i].Value) // ID(2) + Length(1) + Value
	}

	buf := make([]byte, totalLen)
	buf[0] = uint8(count)
	offset := 1

	for i := 0; i < count; i++ {
		param := req.Params[i]
		binary.BigEndian.PutUint16(buf[offset:offset+2], param.ParamID)
		offset += 2

		valueLen := len(param.Value)
		if valueLen > 255 {
			valueLen = 255
		}
		buf[offset] = uint8(valueLen)
		offset++

		copy(buf[offset:offset+valueLen], param.Value)
		offset += valueLen
	}

	return buf
}

// ParamWriteResponse 批量写入参数响应（上行）
type ParamWriteResponse struct {
	Results []ParamWriteResult // 写入结果列表
}

// ParamWriteResult 参数写入结果
type ParamWriteResult struct {
	ParamID uint16 // 参数ID
	Result  uint8  // 结果: 0=成功, 1=失败, 2=参数不存在, 3=值无效
}

// ParseParamWriteResponse 解析参数写入响应
func ParseParamWriteResponse(data []byte) (*ParamWriteResponse, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("param write response too short")
	}

	resp := &ParamWriteResponse{}
	count := data[0]
	offset := 1

	for i := 0; i < int(count) && offset+3 <= len(data); i++ {
		result := ParamWriteResult{
			ParamID: binary.BigEndian.Uint16(data[offset : offset+2]),
			Result:  data[offset+2],
		}
		resp.Results = append(resp.Results, result)
		offset += 3
	}

	return resp, nil
}

// ===== 0x03 参数同步请求 =====

// ParamSyncRequest 参数同步请求（下行）
type ParamSyncRequest struct {
	SyncType uint8 // 同步类型: 0=全部同步, 1=增量同步
}

// EncodeParamSyncRequest 编码参数同步请求
func EncodeParamSyncRequest(req *ParamSyncRequest) []byte {
	return []byte{req.SyncType}
}

// ParamSyncResponse 参数同步响应（上行）
type ParamSyncResponse struct {
	Result   uint8  // 结果: 0=开始同步, 1=同步中, 2=完成, 3=失败
	Progress uint8  // 进度 0-100
	Message  string // 消息
}

// ParseParamSyncResponse 解析参数同步响应
func ParseParamSyncResponse(data []byte) (*ParamSyncResponse, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("param sync response too short")
	}

	resp := &ParamSyncResponse{
		Result:   data[0],
		Progress: data[1],
	}

	if len(data) > 2 {
		resp.Message = string(data[2:])
	}

	return resp, nil
}

// ===== 0x04 参数重置 =====

// ParamResetRequest 参数重置请求（下行）
type ParamResetRequest struct {
	ResetType uint8    // 重置类型: 0=恢复出厂设置, 1=重置指定参数
	ParamIDs  []uint16 // 要重置的参数ID列表（仅当ResetType=1时有效）
}

// EncodeParamResetRequest 编码参数重置请求
func EncodeParamResetRequest(req *ParamResetRequest) []byte {
	if req.ResetType == 0 {
		return []byte{0} // 恢复出厂设置
	}

	count := len(req.ParamIDs)
	if count > 20 {
		count = 20
	}

	buf := make([]byte, 2+count*2)
	buf[0] = 1 // 重置指定参数
	buf[1] = uint8(count)

	offset := 2
	for i := 0; i < count; i++ {
		binary.BigEndian.PutUint16(buf[offset:offset+2], req.ParamIDs[i])
		offset += 2
	}

	return buf
}

// ParamResetResponse 参数重置响应（上行）
type ParamResetResponse struct {
	Result  uint8  // 结果: 0=成功, 1=失败
	Message string // 消息
}

// ParseParamResetResponse 解析参数重置响应
func ParseParamResetResponse(data []byte) (*ParamResetResponse, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("param reset response too short")
	}

	resp := &ParamResetResponse{
		Result: data[0],
	}

	if len(data) > 1 {
		resp.Message = string(data[1:])
	}

	return resp, nil
}

// ===== 辅助函数 =====

// GetParamWriteResultDescription 获取参数写入结果描述
func GetParamWriteResultDescription(result uint8) string {
	switch result {
	case 0:
		return "成功"
	case 1:
		return "失败"
	case 2:
		return "参数不存在"
	case 3:
		return "值无效"
	default:
		return fmt.Sprintf("未知结果(%d)", result)
	}
}

// GetParamSyncResultDescription 获取参数同步结果描述
func GetParamSyncResultDescription(result uint8) string {
	switch result {
	case 0:
		return "开始同步"
	case 1:
		return "同步中"
	case 2:
		return "完成"
	case 3:
		return "失败"
	default:
		return fmt.Sprintf("未知结果(%d)", result)
	}
}
