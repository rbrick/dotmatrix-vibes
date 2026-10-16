package matrix

import (
	"image/color"
	"testing"
	"time"
)

// clockScene records the elapsed time it was rendered with.
type clockScene struct {
	col  color.RGBA
	last *time.Duration
}

func (s clockScene) Render(c *Canvas, elapsed time.Duration) time.Duration {
	*s.last = elapsed
	c.Fill(s.col)
	return -1
}

func TestRotation(t *testing.T) {
	var aT, bT time.Duration
	a := clockScene{color.RGBA{200, 0, 0, 255}, &aT}
	b := clockScene{color.RGBA{0, 0, 200, 255}, &bT}
	r, err := NewRotation(RotationItem{a, 10 * time.Second}, RotationItem{b, 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	c := NewCanvas()
	sec := func(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

	// Middle of the first scene: full brightness, wake up when the fade starts.
	if next := r.Render(c, sec(3)); next != sec(10-3)-RotationFade || c.Pix[0][0] != a.col || aT != sec(3) {
		t.Errorf("t=3s: next %v, pixel %v, scene time %v", next, c.Pix[0][0], aT)
	}
	// Fading out at the end of the first scene.
	if next := r.Render(c, sec(9.8)); next != fadeFrame || c.Pix[0][0].R >= 200 || c.Pix[0][0].R == 0 {
		t.Errorf("t=9.8s: next %v, pixel %v", next, c.Pix[0][0])
	}
	// Second scene, starting from zero.
	r.Render(c, sec(12))
	if c.Pix[0][0] != b.col || bT != sec(2) {
		t.Errorf("t=12s: pixel %v, scene time %v", c.Pix[0][0], bT)
	}
	// Second turn of the first scene carries on from its first turn.
	r.Render(c, sec(15+4))
	if c.Pix[0][0] != a.col || aT != sec(10+4) {
		t.Errorf("t=19s: pixel %v, scene time %v", c.Pix[0][0], aT)
	}
	if i, in := r.Current(sec(30 + 11)); i != 1 || in != sec(1) {
		t.Errorf("Current(41s) = %d, %v", i, in)
	}
}

func TestRotationValidates(t *testing.T) {
	var d time.Duration
	if _, err := NewRotation(); err == nil {
		t.Error("empty rotation accepted")
	}
	if _, err := NewRotation(RotationItem{clockScene{last: &d}, 100 * time.Millisecond}); err == nil {
		t.Error("too-short item accepted")
	}
}
