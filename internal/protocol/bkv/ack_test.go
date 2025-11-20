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

func TestStatusAckFrameMatchesDocSample(t *testing.T) {
	payload := &BKVPayload{
		Cmd:       0x1017,
		FrameSeq:  0,
		GatewayID: "82231214002700",
	}
	bkvAck, err := EncodeBKVStatusAck(payload, true)
	if err != nil {
		t.Fatalf("encode ack error: %v", err)
	}

	frame := Build(0x1000, 0, payload.GatewayID, bkvAck)
	got := hex.EncodeToString(frame)
	want := "fcff002f100000000000008223121400270004010110170a010200000000000000000901038223121400270003010f017efcee"
	if got != want {
		t.Fatalf("doc sample mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestChargingEndAckMatchesDocSample(t *testing.T) {
	payload := &BKVPayload{
		Cmd:       0x1004,
		FrameSeq:  0,
		GatewayID: "82210225000520",
	}
	socket := 1
	port := 0
	bkvAck, err := EncodeBKVChargingEndAck(payload, &socket, &port, true)
	if err != nil {
		t.Fatalf("encode charging-end ack error: %v", err)
	}

	frame := Build(0x1000, 0, payload.GatewayID, bkvAck)
	want := "fcff0037100000000000008221022500052004010110040a010200000000000000000901038221022500052003010f0103014a0103010800c8fcee"
	if got := hex.EncodeToString(frame); got != want {
		t.Fatalf("charging-end ack mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestExceptionAckMatchesDocSample(t *testing.T) {
	payload := &BKVPayload{
		Cmd:       0x1010,
		FrameSeq:  0,
		GatewayID: "82230811001447",
	}
	socket := 2
	bkvAck, err := EncodeBKVExceptionAck(payload, &socket, true)
	if err != nil {
		t.Fatalf("encode exception ack error: %v", err)
	}

	frame := Build(0x1000, 0, payload.GatewayID, bkvAck)
	want := "fcff0033100000000000008223081100144704010110100a010200000000000000000901038223081100144703014a0203010f0119fcee"
	if got := hex.EncodeToString(frame); got != want {
		t.Fatalf("exception ack mismatch:\n got: %s\nwant: %s", got, want)
	}
}
