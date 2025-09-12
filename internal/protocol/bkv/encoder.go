package bkv

// Build 构造一帧 BKV 下行示例帧：fc fe | len | cmd | data | sum
// 注意：此为占位编码，与 parser.go 的 Parse/StreamDecoder 兼容（sum 未严格校验）。
func Build(cmd byte, data []byte) []byte {
    total := 2 + 1 + 1 + len(data) + 1
    buf := make([]byte, 0, total)
    buf = append(buf, magicA...) // 默认使用 FC FE
    buf = append(buf, byte(total))
    buf = append(buf, cmd)
    buf = append(buf, data...)
    buf = append(buf, 0x00)
    return buf
}


