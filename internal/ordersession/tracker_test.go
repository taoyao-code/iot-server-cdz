package ordersession

import (
	"testing"
	"time"
)

type fakeClock struct {
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(0, 0)}
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
}

func TestTrackerPromoteAndLookup(t *testing.T) {
	clock := newFakeClock()
	tracker := NewTracker(WithTTLs(time.Minute, time.Hour), WithNow(clock.Now))

	tracker.TrackPending("dev-1", 0, 1, "ORDER-1", "mode_1")

	session, err := tracker.Promote("dev-1", 0, "0042")
	if err != nil {
		t.Fatalf("promote failed: %v", err)
	}
	if session.OrderNo != "ORDER-1" || session.BusinessNo != "0042" {
		t.Fatalf("unexpected session: %+v", session)
	}

	if _, ok := tracker.Lookup("dev-1", 0); !ok {
		t.Fatalf("lookup by port failed")
	}

	if _, ok := tracker.LookupByBusiness("dev-1", "0042"); !ok {
		t.Fatalf("lookup by business failed")
	}

	tracker.Clear("dev-1", 0)
	if _, ok := tracker.Lookup("dev-1", 0); ok {
		t.Fatalf("session should be cleared")
	}
}

func TestTrackerPromoteMissing(t *testing.T) {
	tracker := NewTracker()
	if _, err := tracker.Promote("dev", 0, "0001"); err != ErrPendingNotFound {
		t.Fatalf("expected ErrPendingNotFound, got %v", err)
	}
}

func TestTrackerTTLExpiry(t *testing.T) {
	clock := newFakeClock()
	tracker := NewTracker(WithTTLs(time.Second, 2*time.Second), WithNow(clock.Now))

	tracker.TrackPending("dev", 0, 0, "ORDER", "mode")
	clock.Advance(1500 * time.Millisecond)

	if _, err := tracker.Promote("dev", 0, "0002"); err != ErrPendingExpired {
		t.Fatalf("expected ErrPendingExpired, got %v", err)
	}

	tracker.TrackPending("dev", 0, 0, "ORDER", "mode")
	tracker.Promote("dev", 0, "0003")

	clock.Advance(3 * time.Second)
	if _, ok := tracker.LookupByBusiness("dev", "0003"); ok {
		t.Fatalf("session should expire after active TTL")
	}
}
