// Package gamepad watches Linux game controllers and turns their buttons into
// simple actions. Controllers can be connected before or after the program
// starts; no controller-specific configuration is required.
package gamepad

import "context"

// Action is a controller action understood by games.
type Action int

const (
	Left Action = iota
	Right
	Select
	Start
)

// Listen watches controllers until ctx is cancelled. Errors opening an input
// device are ignored so hot-plugging and composite controllers remain robust.
func Listen(ctx context.Context, action func(Action)) {
	listen(ctx, action)
}
