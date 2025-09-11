package bkv

// Frame BKV 最小帧结构（骨架）
// 约定：magic[2]=0xFC,0xFE | len(1) | cmd(1) | data[n] | sum(1)
// 实际协议可能为TLV/变长，此处仅占位用于集成与测试
type Frame struct {
    Cmd  byte
    Data []byte
}

var magicA = []byte{0xFC, 0xFE}
var magicB = []byte{0xFC, 0xFF}


