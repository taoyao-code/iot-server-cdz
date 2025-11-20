package bkv

import (
	"encoding/hex"
	"testing"
)

func TestEncodeBKVStatusAck_Success(t *testing.T) {
	payload := &BKVPayload{
		Cmd:       0x1017,
		FrameSeq:  0,
		GatewayID: "82231214002700",
	}

	data, err := EncodeBKVStatusAck(payload, true)
	if err != nil {
		t.Fatalf("encode status ack error: %v", err)
	}

	got := hex.EncodeToString(data)
	expect := "04010110170a010200000000000000000901038223121400270003010f01"
	if got != expect {
		t.Fatalf("status ack mismatch:\n got: %s\nwant: %s", got, expect)
	}
}

func TestEncodeBKVStatusAck_Fail(t *testing.T) {
	payload := &BKVPayload{
		Cmd:       0x1017,
		FrameSeq:  0x0102030405060708,
		GatewayID: "00112233445566",
	}

	data, err := EncodeBKVStatusAck(payload, false)
	if err != nil {
		t.Fatalf("encode status ack error: %v", err)
	}

	hexStr := hex.EncodeToString(data)
	if hexStr[len(hexStr)-2:] != "00" {
		t.Fatalf("expect failure ack tail 00, got %s", hexStr[len(hexStr)-2:])
	}
}
