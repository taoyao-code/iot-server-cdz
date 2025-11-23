package app

import "context"

// BillingHookFunc 便捷包装函数
type BillingHookFunc struct {
	OnStart func(ctx context.Context, biz string, port int32, cardNo *string) error
	OnEnd   func(ctx context.Context, biz string, port int32, amountCent *int64, energyKwh01 int32, durationSec int32) error
}

func (b *BillingHookFunc) OnSessionStarted(ctx context.Context, biz string, port int32, cardNo *string) error {
	if b == nil || b.OnStart == nil {
		return nil
	}
	return b.OnStart(ctx, biz, port, cardNo)
}

func (b *BillingHookFunc) OnSessionEnded(ctx context.Context, biz string, port int32, amountCent *int64, energyKwh01 int32, durationSec int32) error {
	if b == nil || b.OnEnd == nil {
		return nil
	}
	return b.OnEnd(ctx, biz, port, amountCent, energyKwh01, durationSec)
}
