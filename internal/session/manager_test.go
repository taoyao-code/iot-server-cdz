package session

import (
	"testing"
	"time"
)

func TestManager_OnHeartbeat_IsOnline(t *testing.T) {
	m := New(2 * time.Second)
	now := time.Now()
	if m.IsOnline("A", now) {
		t.Fatalf("expected offline initially")
	}
	m.OnHeartbeat("A", now)
	if !m.IsOnline("A", now) {
		t.Fatalf("expected online after heartbeat")
	}
	if m.IsOnline("B", now) {
		t.Fatalf("other device should be offline")
	}
}

func TestManager_Timeout(t *testing.T) {
	m := New(500 * time.Millisecond)
	ts := time.Now()
	m.OnHeartbeat("X", ts)
	if !m.IsOnline("X", ts.Add(400*time.Millisecond)) {
		t.Fatalf("should still be online before timeout")
	}
	if m.IsOnline("X", ts.Add(600*time.Millisecond)) {
		t.Fatalf("should be offline after timeout")
	}
}
