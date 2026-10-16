package matrix

import (
	"errors"
	"time"
)

// RotationItem is one scene of a Rotation and how long it stays up.
type RotationItem struct {
	Scene    Scene
	Duration time.Duration
}

// Rotation shows each of its scenes in turn, fading between them, and
// starts again after the last. A scene's elapsed time only advances while it
// is on screen, so animations carry on where they left off: a planet tour
// shows the next planets on its following turn rather than starting over.
type Rotation struct {
	Items []RotationItem
	cycle time.Duration
}

// RotationFade is how long a scene takes to fade out, and the next to fade
// in.
const RotationFade = 400 * time.Millisecond

// NewRotation checks that there is something to show and every scene stays
// up long enough to fade in and out.
func NewRotation(items ...RotationItem) (*Rotation, error) {
	if len(items) == 0 {
		return nil, errors.New("rotation: no scenes")
	}
	r := &Rotation{Items: items}
	for _, it := range items {
		if it.Scene == nil {
			return nil, errors.New("rotation: nil scene")
		}
		if it.Duration < 2*RotationFade {
			return nil, errors.New("rotation: each scene needs at least a second")
		}
		r.cycle += it.Duration
	}
	return r, nil
}

// Current returns the index of the scene on screen at elapsed, and how long
// it has been up.
func (r *Rotation) Current(elapsed time.Duration) (int, time.Duration) {
	in := elapsed % r.cycle
	for i, it := range r.Items {
		if in < it.Duration {
			return i, in
		}
		in -= it.Duration
	}
	return 0, 0 // unreachable
}

// fadeFrame is the frame interval while fading.
const fadeFrame = time.Second / 30

// Render implements Scene.
func (r *Rotation) Render(c *Canvas, elapsed time.Duration) time.Duration {
	i, in := r.Current(elapsed)
	it := r.Items[i]
	// The scene's own clock: every full turn it has had, plus this one.
	own := time.Duration(elapsed/r.cycle)*it.Duration + in
	next := it.Scene.Render(c, own)

	left := it.Duration - in // until the next scene
	if len(r.Items) == 1 {
		return next
	}
	level := 1.0
	switch {
	case in < RotationFade:
		level = float64(in) / float64(RotationFade)
	case left <= RotationFade:
		level = float64(left) / float64(RotationFade)
	}
	if level < 1 {
		c.dim(level)
		return fadeFrame
	}
	// Wake up in time to start fading out.
	untilFade := left - RotationFade
	if next < 0 || next > untilFade {
		next = untilFade
	}
	return next
}

// dim scales every pixel's brightness by f (0-1).
func (c *Canvas) dim(f float64) {
	for y := range c.Pix {
		for x := range c.Pix[y] {
			p := &c.Pix[y][x]
			p.R, p.G, p.B = uint8(float64(p.R)*f+0.5), uint8(float64(p.G)*f+0.5), uint8(float64(p.B)*f+0.5)
		}
	}
}
