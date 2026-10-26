//go:build !linux

package gamepad

import "context"

func listen(ctx context.Context, action func(Action)) {
	<-ctx.Done()
}
