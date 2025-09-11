package ap3000

import "context"

// 为 fakeRepo 补充新增接口方法，便于编译通过
func (f *fakeRepo) UpsertOrderProgress(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, status int, powerW01 *int) error {
	return f.error
}

func (f *fakeRepo) SettleOrder(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, reason int) error {
	return f.error
}
