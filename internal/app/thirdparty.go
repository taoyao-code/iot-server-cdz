package app

import (
	"context"

	"github.com/taoyao-code/iot-server/internal/thirdparty"
)

// NewPusherIfEnabled 根据配置创建第三方推送器
func NewPusherIfEnabled(webhookURL, secret string) (pusher interface {
	SendJSON(ctx context.Context, endpoint string, payload any) (int, []byte, error)
}, url string,
) {
	if webhookURL != "" && secret != "" {
		return thirdparty.NewPusher(nil, "", secret), webhookURL
	}
	return nil, ""
}
