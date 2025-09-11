package ap3000

import "encoding/binary"

// Build 构造一帧 AP3000 下行数据帧（与 Parse 对应）。
// 说明：len 字段与当前解析实现保持一致，等于整个帧的总长度。
func Build(phy string, msgID uint16, cmd byte, data []byte) []byte {
    phyb := []byte(phy)
    total := 3 + 2 + 1 + len(phyb) + 2 + 1 + len(data) + 2
    buf := make([]byte, 0, total)
    // magic
    buf = append(buf, magic...)
    // total length (little-endian)
    l := make([]byte, 2)
    binary.LittleEndian.PutUint16(l, uint16(total))
    buf = append(buf, l...)
    // phy length + phy bytes
    buf = append(buf, byte(len(phyb)))
    buf = append(buf, phyb...)
    // msgID
    mid := make([]byte, 2)
    binary.LittleEndian.PutUint16(mid, msgID)
    buf = append(buf, mid...)
    // cmd
    buf = append(buf, cmd)
    // payload
    buf = append(buf, data...)
    // checksum (low 16 bits sum of all previous bytes)
    sumLE := make([]byte, 2)
    binary.LittleEndian.PutUint16(sumLE, checksum16(buf))
    buf = append(buf, sumLE...)
    return buf
}


