package ap3000

import (
	"encoding/binary"
	"testing"
)

func makeFrame(phy string, msgID uint16, cmd byte, data []byte) []byte {
	phyb := []byte(phy)
	// magic + len + phyLen + phy + msgId + cmd + data + sum
	total := 3 + 2 + 1 + len(phyb) + 2 + 1 + len(data) + 2
	buf := make([]byte, 0, total)
	buf = append(buf, magic...)
	l := make([]byte, 2)
	binary.LittleEndian.PutUint16(l, uint16(total))
	buf = append(buf, l...)
	buf = append(buf, byte(len(phyb)))
	buf = append(buf, phyb...)
	mid := make([]byte, 2)
	binary.LittleEndian.PutUint16(mid, msgID)
	buf = append(buf, mid...)
	buf = append(buf, cmd)
	buf = append(buf, data...)
	// 计算校验和（低16位），覆盖除末尾校验字段以外的所有字节
	s := checksum16(buf)
	sb := make([]byte, 2)
	binary.LittleEndian.PutUint16(sb, s)
	buf = append(buf, sb...)
	return buf
}

func TestParse_OK(t *testing.T) {
	raw := makeFrame("ABC123", 0x1234, 0x20, []byte{0x01, 0x02})
	fr, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fr.PhyID != "ABC123" || fr.MsgID != 0x1234 || fr.Cmd != 0x20 {
		t.Fatalf("unexpected frame: %+v", fr)
	}
	if len(fr.Data) != 2 {
		t.Fatalf("unexpected data len: %d", len(fr.Data))
	}
}

func TestParse_InvalidMagic(t *testing.T) {
	raw := makeFrame("X", 1, 1, nil)
	raw[0] = 0x00
	if _, err := Parse(raw); err == nil {
		t.Fatalf("expected error but nil")
	}
}
