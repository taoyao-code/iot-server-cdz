package ordersession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	redis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var (
	ErrPendingNotFound  = errors.New("pending session not found")
	ErrPendingExpired   = errors.New("pending session expired")
	errRedisUnavailable = errors.New("ordersession redis unavailable")
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
	pending     sync.Map
	active      sync.Map
	bizIndex    sync.Map
	redis       redis.UniversalClient
	redisPrefix string

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
		pendingTTL:  defaultPendingTTL,
		activeTTL:   defaultActiveTTL,
		observer:    NopObserver(),
		now:         time.Now,
		redisPrefix: "ordersession",
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

func WithRedisClient(client redis.UniversalClient, prefix string) Option {
	return func(t *Tracker) {
		if client == nil {
			return
		}
		t.redis = client
		t.redisPrefix = strings.TrimSpace(prefix)
		if t.redisPrefix == "" {
			t.redisPrefix = "ordersession"
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
	if err := t.storePendingRedis(session); err != nil {
		t.observer.Record("pending_track", "redis_error")
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
	if t.useRedis() {
		if active, err := t.promoteRedis(deviceID, portNo, businessNo, now); err != errRedisUnavailable {
			if err == nil {
				t.pending.Delete(key)
				t.maybeSweepPending(now)
				t.cacheActive(deviceID, portNo, active)
				t.observer.Record("promote", "ok")
				return active, nil
			}
			if errors.Is(err, ErrPendingNotFound) {
				t.maybeSweepPending(now)
				t.observer.Record("promote", "missing")
				return nil, err
			}
			if errors.Is(err, ErrPendingExpired) {
				t.pending.Delete(key)
				t.maybeSweepPending(now)
				t.observer.Record("promote", "expired")
				return nil, err
			}
			return nil, err
		}
	}

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
	t.cacheActive(deviceID, portNo, active)
	t.observer.Record("promote", "ok")
	return active, nil
}

func (t *Tracker) Lookup(deviceID string, portNo int) (*ActiveSession, bool) {
	now := t.now()
	t.maybeSweepActive(now)
	if t.useRedis() {
		if session, status, err := t.lookupRedis(deviceID, portNo, now); err != errRedisUnavailable {
			if err != nil {
				return nil, false
			}
			switch status {
			case "hit":
				t.cacheActive(deviceID, portNo, session)
				t.observer.Record("lookup_port", "hit")
				return session, true
			case "expired":
				t.observer.Record("lookup_port", "expired")
				return nil, false
			default:
				t.observer.Record("lookup_port", "miss")
				return nil, false
			}
		}
	}

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
	if t.useRedis() {
		if session, status, err := t.lookupByBizRedis(deviceID, businessNo, now); err != errRedisUnavailable {
			if err != nil {
				return nil, false
			}
			switch status {
			case "hit":
				t.cacheActive(deviceID, int(session.PortNo), session)
				t.observer.Record("lookup_biz", "hit")
				return session, true
			case "expired":
				t.observer.Record("lookup_biz", "expired")
				return nil, false
			default:
				t.observer.Record("lookup_biz", "miss")
				return nil, false
			}
		}
	}

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
	removedActive := false
	key := sessionKey(deviceID, portNo)
	if val, ok := t.active.Load(key); ok {
		session := val.(*ActiveSession)
		t.active.Delete(key)
		t.bizIndex.Delete(bizKey(session.DeviceID, session.BusinessNo))
		removedActive = true
	}
	if t.useRedis() && t.clearActiveRedis(deviceID, portNo) {
		removedActive = true
	}
	if removedActive {
		t.observer.Record("clear", "active")
	}
	t.ClearPending(deviceID, portNo)
}

func (t *Tracker) ClearByBusiness(deviceID, businessNo string) {
	removedActive := false
	if t.useRedis() && t.clearBizRedis(deviceID, businessNo) {
		removedActive = true
	}
	bizKey := bizKey(deviceID, businessNo)
	if rawKey, ok := t.bizIndex.Load(bizKey); ok {
		if val, ok := t.active.Load(rawKey.(string)); ok {
			t.deleteActive(rawKey.(string), val.(*ActiveSession))
			removedActive = true
		}
	}
	if removedActive {
		t.observer.Record("clear", "active")
	}
}

func (t *Tracker) ClearPending(deviceID string, portNo int) {
	removed := false
	if t.useRedis() && t.clearPendingRedis(deviceID, portNo) {
		removed = true
	}
	key := sessionKey(deviceID, portNo)
	if _, ok := t.pending.Load(key); ok {
		t.pending.Delete(key)
		removed = true
	}
	if removed {
		t.observer.Record("clear", "pending")
	}
}

func (t *Tracker) LoadPendingFromRedis() error {
	if !t.useRedis() {
		return nil
	}
	ctx := context.Background()
	iter := t.redis.Scan(ctx, 0, fmt.Sprintf("%s:pending:*", t.redisPrefix), 0).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		data, err := t.redis.Get(ctx, key).Bytes()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			return err
		}
		session := new(PendingSession)
		if err := json.Unmarshal(data, session); err != nil {
			zap.L().Warn("ordersession: skip invalid pending session from redis", zap.String("key", key), zap.Error(err))
			continue
		}
		t.pending.Store(sessionKey(session.DeviceID, session.PortNo), session)
	}
	if err := iter.Err(); err != nil {
		return err
	}
	return nil
}

func (t *Tracker) LoadActiveFromRedis() error {
	if !t.useRedis() {
		return nil
	}
	ctx := context.Background()
	iter := t.redis.Scan(ctx, 0, fmt.Sprintf("%s:active:*", t.redisPrefix), 0).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		data, err := t.redis.Get(ctx, key).Bytes()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			return err
		}
		session := new(ActiveSession)
		if err := json.Unmarshal(data, session); err != nil {
			zap.L().Warn("ordersession: skip invalid active session from redis", zap.String("key", key), zap.Error(err))
			continue
		}
		session.BusinessNo = normalizeBusinessNo(session.BusinessNo)
		sKey := sessionKey(session.DeviceID, session.PortNo)
		t.active.Store(sKey, session)
		t.bizIndex.Store(bizKey(session.DeviceID, session.BusinessNo), sKey)
	}
	if err := iter.Err(); err != nil {
		return err
	}
	return nil
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

func (t *Tracker) useRedis() bool {
	return t != nil && t.redis != nil
}

func (t *Tracker) redisPendingKey(deviceID string, portNo int) string {
	return fmt.Sprintf("%s:pending:%s", t.redisPrefix, sessionKey(deviceID, portNo))
}

func (t *Tracker) redisActiveKey(deviceID string, portNo int) string {
	return fmt.Sprintf("%s:active:%s", t.redisPrefix, sessionKey(deviceID, portNo))
}

func (t *Tracker) redisBizKey(deviceID, business string) string {
	return fmt.Sprintf("%s:biz:%s", t.redisPrefix, bizKey(deviceID, business))
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

func (t *Tracker) cacheActive(deviceID string, portNo int, active *ActiveSession) {
	if active == nil {
		return
	}
	key := sessionKey(deviceID, portNo)
	t.active.Store(key, active)
	t.bizIndex.Store(bizKey(deviceID, active.BusinessNo), key)
}

func (t *Tracker) storePendingRedis(session *PendingSession) error {
	if !t.useRedis() || session == nil {
		return nil
	}
	payload, err := json.Marshal(session)
	if err != nil {
		return err
	}
	ttl := t.pendingTTL
	if ttl < 0 {
		ttl = 0
	}
	return t.redis.Set(context.Background(), t.redisPendingKey(session.DeviceID, session.PortNo), payload, ttl).Err()
}

func (t *Tracker) promoteRedis(deviceID string, portNo int, businessNo string, now time.Time) (*ActiveSession, error) {
	if !t.useRedis() {
		return nil, errRedisUnavailable
	}
	ctx := context.Background()
	key := t.redisPendingKey(deviceID, portNo)
	data, err := t.redis.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, ErrPendingNotFound
	}
	if err != nil {
		return nil, errRedisUnavailable
	}
	var pending PendingSession
	if unmarshalErr := json.Unmarshal(data, &pending); unmarshalErr != nil {
		_ = t.redis.Del(ctx, key)
		return nil, ErrPendingNotFound
	}
	if pending.expired(t.pendingTTL, now) {
		_ = t.redis.Del(ctx, key)
		return nil, ErrPendingExpired
	}
	active := &ActiveSession{
		PendingSession: pending,
		BusinessNo:     normalizeBusinessNo(businessNo),
		AckAt:          now,
	}
	payload, err := json.Marshal(active)
	if err != nil {
		return nil, errRedisUnavailable
	}
	activeKey := t.redisActiveKey(deviceID, portNo)
	biz := t.redisBizKey(deviceID, active.BusinessNo)
	_, err = t.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Del(ctx, key)
		pipe.Set(ctx, activeKey, payload, t.activeTTL)
		pipe.Set(ctx, biz, sessionKey(deviceID, portNo), t.activeTTL)
		return nil
	})
	if err != nil {
		return nil, errRedisUnavailable
	}
	return active, nil
}

func (t *Tracker) lookupRedis(deviceID string, portNo int, now time.Time) (*ActiveSession, string, error) {
	if !t.useRedis() {
		return nil, "", errRedisUnavailable
	}
	ctx := context.Background()
	key := t.redisActiveKey(deviceID, portNo)
	data, err := t.redis.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, "miss", nil
	}
	if err != nil {
		return nil, "", errRedisUnavailable
	}
	var session ActiveSession
	if unmarshalErr := json.Unmarshal(data, &session); unmarshalErr != nil {
		_ = t.redis.Del(ctx, key)
		_ = t.redis.Del(ctx, t.redisBizKey(deviceID, session.BusinessNo))
		return nil, "miss", nil
	}
	if session.expired(t.activeTTL, now) {
		_ = t.redis.Del(ctx, key)
		_ = t.redis.Del(ctx, t.redisBizKey(deviceID, session.BusinessNo))
		return nil, "expired", nil
	}
	return &session, "hit", nil
}

func (t *Tracker) lookupByBizRedis(deviceID, businessNo string, now time.Time) (*ActiveSession, string, error) {
	if !t.useRedis() {
		return nil, "", errRedisUnavailable
	}
	ctx := context.Background()
	bizKey := t.redisBizKey(deviceID, businessNo)
	sKey, err := t.redis.Get(ctx, bizKey).Result()
	if err == redis.Nil {
		return nil, "miss", nil
	}
	if err != nil {
		return nil, "", errRedisUnavailable
	}
	port := parsePortFromSessionKey(sKey)
	return t.lookupRedis(deviceID, port, now)
}

func (t *Tracker) clearActiveRedis(deviceID string, portNo int) bool {
	if !t.useRedis() {
		return false
	}
	ctx := context.Background()
	activeKey := t.redisActiveKey(deviceID, portNo)
	existed := false
	if data, err := t.redis.Get(ctx, activeKey).Bytes(); err == nil {
		existed = true
		var session ActiveSession
		if json.Unmarshal(data, &session) == nil {
			_ = t.redis.Del(ctx, t.redisBizKey(deviceID, session.BusinessNo))
		}
	}
	_ = t.redis.Del(ctx, activeKey)
	return existed
}

func (t *Tracker) clearBizRedis(deviceID, business string) bool {
	if !t.useRedis() {
		return false
	}
	ctx := context.Background()
	biz := t.redisBizKey(deviceID, business)
	sKey, err := t.redis.Get(ctx, biz).Result()
	if err == redis.Nil {
		_ = t.redis.Del(ctx, biz)
		return false
	}
	if err != nil {
		return false
	}
	port := parsePortFromSessionKey(sKey)
	_ = t.redis.Del(ctx, t.redisActiveKey(deviceID, port))
	_ = t.redis.Del(ctx, biz)
	return true
}

func parsePortFromSessionKey(key string) int {
	idx := strings.LastIndex(key, ":")
	if idx < 0 {
		return 0
	}
	port, err := strconv.Atoi(key[idx+1:])
	if err != nil {
		return 0
	}
	return port
}

func (t *Tracker) clearPendingRedis(deviceID string, portNo int) bool {
	if !t.useRedis() {
		return false
	}
	ctx := context.Background()
	res, err := t.redis.Del(ctx, t.redisPendingKey(deviceID, portNo)).Result()
	return err == nil && res > 0
}
