package ordersession

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrPendingNotFound = errors.New("pending session not found")
	ErrPendingExpired  = errors.New("pending session expired")
)

type Observer interface {
	Record(operation, status string)
}

type ObserverFunc func(operation, status string)

func (f ObserverFunc) Record(operation, status string) {
	if f != nil {
		f(operation, status)
	}
}

func NopObserver() Observer {
	return ObserverFunc(func(string, string) {})
}

type PendingSession struct {
	DeviceID   string
	PortNo     int
	SocketNo   int
	OrderNo    string
	ChargeMode string
	CreatedAt  time.Time
}

func (p *PendingSession) expired(ttl time.Duration, now time.Time) bool {
	if ttl <= 0 {
		return false
	}
	return now.Sub(p.CreatedAt) > ttl
}

type ActiveSession struct {
	PendingSession
	BusinessNo string
	AckAt      time.Time
}

func (a *ActiveSession) expired(ttl time.Duration, now time.Time) bool {
	if ttl <= 0 {
		return false
	}
	return now.Sub(a.AckAt) > ttl
}

type Tracker struct {
	pending  sync.Map
	active   sync.Map
	bizIndex sync.Map

	pendingTTL time.Duration
	activeTTL  time.Duration
	observer   Observer
	now        func() time.Time

	lastPendingSweep int64
	lastActiveSweep  int64
}

type Option func(*Tracker)

const (
	defaultPendingTTL = 5 * time.Minute
	defaultActiveTTL  = 4 * time.Hour
)

func NewTracker(opts ...Option) *Tracker {
	t := &Tracker{
		pendingTTL: defaultPendingTTL,
		activeTTL:  defaultActiveTTL,
		observer:   NopObserver(),
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

func WithTTLs(pendingTTL, activeTTL time.Duration) Option {
	return func(t *Tracker) {
		if pendingTTL > 0 {
			t.pendingTTL = pendingTTL
		}
		if activeTTL > 0 {
			t.activeTTL = activeTTL
		}
	}
}

func WithObserver(observer Observer) Option {
	return func(t *Tracker) {
		if observer != nil {
			t.observer = observer
		}
	}
}

func WithNow(now func() time.Time) Option {
	return func(t *Tracker) {
		if now != nil {
			t.now = now
		}
	}
}

func (t *Tracker) TrackPending(deviceID string, portNo, socketNo int, orderNo, chargeMode string) *PendingSession {
	now := t.now()
	t.maybeSweepPending(now)

	session := &PendingSession{
		DeviceID:   strings.TrimSpace(deviceID),
		PortNo:     portNo,
		SocketNo:   socketNo,
		OrderNo:    strings.TrimSpace(orderNo),
		ChargeMode: chargeMode,
		CreatedAt:  now,
	}
	key := sessionKey(deviceID, portNo)
	t.pending.Store(key, session)
	t.observer.Record("pending_track", "ok")
	return session
}

func (t *Tracker) Promote(deviceID string, portNo int, businessNo string) (*ActiveSession, error) {
	now := t.now()
	t.maybeSweepActive(now)

	key := sessionKey(deviceID, portNo)
	pendingVal, ok := t.pending.Load(key)
	if !ok {
		t.maybeSweepPending(now)
		t.observer.Record("promote", "missing")
		return nil, ErrPendingNotFound
	}
	pending := pendingVal.(*PendingSession)
	if pending.expired(t.pendingTTL, now) {
		t.pending.Delete(key)
		t.observer.Record("promote", "expired")
		return nil, ErrPendingExpired
	}
	t.pending.Delete(key)
	t.maybeSweepPending(now)

	active := &ActiveSession{
		PendingSession: *pending,
		BusinessNo:     normalizeBusinessNo(businessNo),
		AckAt:          now,
	}
	t.active.Store(key, active)
	t.bizIndex.Store(bizKey(deviceID, active.BusinessNo), key)
	t.observer.Record("promote", "ok")
	return active, nil
}

func (t *Tracker) Lookup(deviceID string, portNo int) (*ActiveSession, bool) {
	now := t.now()
	t.maybeSweepActive(now)

	key := sessionKey(deviceID, portNo)
	val, ok := t.active.Load(key)
	if !ok {
		t.observer.Record("lookup_port", "miss")
		return nil, false
	}
	session := val.(*ActiveSession)
	if session.expired(t.activeTTL, now) {
		t.deleteActive(key, session)
		t.observer.Record("lookup_port", "expired")
		return nil, false
	}
	t.observer.Record("lookup_port", "hit")
	return session, true
}

func (t *Tracker) LookupByBusiness(deviceID, businessNo string) (*ActiveSession, bool) {
	now := t.now()
	t.maybeSweepActive(now)

	bizKey := bizKey(deviceID, businessNo)
	rawKey, ok := t.bizIndex.Load(bizKey)
	if !ok {
		t.observer.Record("lookup_biz", "miss")
		return nil, false
	}
	sessionKey := rawKey.(string)
	val, ok := t.active.Load(sessionKey)
	if !ok {
		t.bizIndex.Delete(bizKey)
		t.observer.Record("lookup_biz", "miss")
		return nil, false
	}
	session := val.(*ActiveSession)
	if session.expired(t.activeTTL, now) {
		t.deleteActive(sessionKey, session)
		t.observer.Record("lookup_biz", "expired")
		return nil, false
	}
	t.observer.Record("lookup_biz", "hit")
	return session, true
}

func (t *Tracker) Clear(deviceID string, portNo int) {
	key := sessionKey(deviceID, portNo)
	if val, ok := t.active.Load(key); ok {
		t.deleteActive(key, val.(*ActiveSession))
		t.observer.Record("clear", "active")
	}
	if _, ok := t.pending.Load(key); ok {
		t.pending.Delete(key)
		t.observer.Record("clear", "pending")
	}
}

func (t *Tracker) ClearByBusiness(deviceID, businessNo string) {
	bizKey := bizKey(deviceID, businessNo)
	if rawKey, ok := t.bizIndex.Load(bizKey); ok {
		if val, ok := t.active.Load(rawKey.(string)); ok {
			t.deleteActive(rawKey.(string), val.(*ActiveSession))
			t.observer.Record("clear", "active")
		}
	}
}

func (t *Tracker) deleteActive(key string, session *ActiveSession) {
	t.active.Delete(key)
	t.bizIndex.Delete(bizKey(session.DeviceID, session.BusinessNo))
}

func (t *Tracker) maybeSweepPending(now time.Time) {
	if t.pendingTTL <= 0 {
		return
	}
	last := time.Unix(0, atomic.LoadInt64(&t.lastPendingSweep))
	if now.Sub(last) < t.pendingTTL {
		return
	}
	t.pending.Range(func(key, value any) bool {
		session := value.(*PendingSession)
		if session.expired(t.pendingTTL, now) {
			t.pending.Delete(key)
			t.observer.Record("pending_track", "expired_cleanup")
		}
		return true
	})
	atomic.StoreInt64(&t.lastPendingSweep, now.UnixNano())
}

func (t *Tracker) maybeSweepActive(now time.Time) {
	if t.activeTTL <= 0 {
		return
	}
	last := time.Unix(0, atomic.LoadInt64(&t.lastActiveSweep))
	if now.Sub(last) < t.activeTTL {
		return
	}
	t.active.Range(func(key, value any) bool {
		session := value.(*ActiveSession)
		if session.expired(t.activeTTL, now) {
			t.deleteActive(key.(string), session)
			t.observer.Record("active_cleanup", "ttl")
		}
		return true
	})
	atomic.StoreInt64(&t.lastActiveSweep, now.UnixNano())
}

func sessionKey(deviceID string, portNo int) string {
	return fmt.Sprintf("%s:%d", strings.TrimSpace(deviceID), portNo)
}

func bizKey(deviceID, business string) string {
	return fmt.Sprintf("%s:%s", strings.TrimSpace(deviceID), normalizeBusinessNo(business))
}

func normalizeBusinessNo(biz string) string {
	s := strings.TrimSpace(biz)
	if s == "" {
		return ""
	}
	return strings.ToUpper(s)
}
