package ap3000

// Frame AP3000 最小帧结构（骨架）
// 布局（最简约定）：
// magic[3] 'D”N”Y' | lenLE[2] | phyLen[1] | phyId[phyLen] | msgIdLE[2] | cmd[1] | data[..] | sumLE[2]
type Frame struct {
	PhyID string
	MsgID uint16
	Cmd   uint8
	Data  []byte
}

var magic = []byte{0x44, 0x4E, 0x59} // 'D''N''Y'
