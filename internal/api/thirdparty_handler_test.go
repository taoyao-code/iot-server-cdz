package api

import (
	"context"
	"fmt"
	"testing"

	"github.com/taoyao-code/iot-server/internal/coremodel"
	"github.com/taoyao-code/iot-server/internal/ordersession"
	"go.uber.org/zap"
)

type fakeDriver struct {
	last *coremodel.CoreCommand
}

func (f *fakeDriver) SendCoreCommand(_ context.Context, cmd *coremodel.CoreCommand) error {
	f.last = cmd
	return nil
}

func TestResolveBusinessNoForCommand(t *testing.T) {
	tracker := ordersession.NewTracker()
	h := &ThirdPartyHandler{orderTracker: tracker}

	const (
		deviceID = "GW-UNIT-01"
		portNo   = 0
		orderNo  = "ORDER-XYZ"
		bizHex   = "00AB"
	)

	tracker.TrackPending(deviceID, portNo, 1, orderNo, "mode_1")
	if _, err := tracker.Promote(deviceID, portNo, bizHex); err != nil {
		t.Fatalf("promote session failed: %v", err)
	}

	biz, fromTracker := h.resolveBusinessNoForCommand(deviceID, portNo, orderNo)
	if !fromTracker {
		t.Fatalf("expected tracker sourced business number")
	}
	if biz != bizHex {
		t.Fatalf("unexpected business number: want=%s got=%s", bizHex, biz)
	}

	mismatchBiz, source := h.resolveBusinessNoForCommand(deviceID, portNo, "OTHER")
	if source {
		t.Fatalf("expected mismatch order fallback")
	}
	expectedFallback := fmt.Sprintf("%04X", deriveBusinessNo("OTHER"))
	if mismatchBiz != expectedFallback {
		t.Fatalf("unexpected fallback business no: want=%s got=%s", expectedFallback, mismatchBiz)
	}
}

func TestSendStartChargeViaDriverSetsBusinessNo(t *testing.T) {
	driver := &fakeDriver{}
	h := &ThirdPartyHandler{
		driverCmd:    driver,
		orderTracker: nil,
		logger:       zap.NewNop(),
	}

	err := h.sendStartChargeViaDriver(context.Background(), "GW-START", 1, 0, "ORDER-START", 1, 30)
	if err != nil {
		t.Fatalf("sendStartChargeViaDriver returned error: %v", err)
	}
	if driver.last == nil {
		t.Fatalf("expected driver command to be captured")
	}
	if driver.last.BusinessNo == nil || *driver.last.BusinessNo == "" {
		t.Fatalf("expected start command to include business number")
	}
}

func TestSendStopChargeViaDriverRequiresActiveSession(t *testing.T) {
	driver := &fakeDriver{}
	tracker := ordersession.NewTracker()
	h := &ThirdPartyHandler{
		driverCmd:    driver,
		orderTracker: tracker,
		logger:       zap.NewNop(),
	}

	if err := h.sendStopChargeViaDriver(context.Background(), "GW-STOP", 1, 0, "ORDER-STOP"); err == nil {
		t.Fatalf("expected error when no active session")
	}

	tracker.TrackPending("GW-STOP", 0, 1, "ORDER-STOP", "mode_1")
	if _, err := tracker.Promote("GW-STOP", 0, "00AF"); err != nil {
		t.Fatalf("promote session failed: %v", err)
	}

	if err := h.sendStopChargeViaDriver(context.Background(), "GW-STOP", 1, 0, "ORDER-STOP"); err != nil {
		t.Fatalf("unexpected error with session: %v", err)
	}
	if driver.last == nil || driver.last.BusinessNo == nil || string(*driver.last.BusinessNo) != "00AF" {
		t.Fatalf("expected stop command to include tracker business number")
	}
}
