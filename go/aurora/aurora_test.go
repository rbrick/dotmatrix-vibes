package aurora

import (
	"image/color"
	"testing"
	"time"

	"dotmatrix/matrix"
)

func TestNew(t *testing.T) {
	if s, err := New(Options{}); err != nil || s.Speed != 1 {
		t.Fatalf("New(defaults) = %+v, %v", s, err)
	}
	for _, sp := range []float64{-1, 11} {
		if _, err := New(Options{Speed: sp}); err == nil {
			t.Errorf("New(speed %g) succeeded", sp)
		}
	}
}

// Over a minute the sky should show strong red, green and blue somewhere,
// and keep moving.
func TestColoursAndMotion(t *testing.T) {
	s, _ := New(Options{})
	var maxR, maxG, maxB uint8
	var prev *matrix.Canvas
	for i := 0; i < 60; i++ {
		c := matrix.NewCanvas()
		if next := s.Render(c, time.Duration(i)*time.Second); next != frameInterval {
			t.Fatalf("next frame in %v, want %v", next, frameInterval)
		}
		for y := 0; y < shore; y++ {
			for x := 0; x < matrix.Width; x++ {
				p := c.Pix[y][x]
				// Count a channel only where it clearly dominates, so white
				// stars don't count as colour.
				if p.R > p.G+40 && p.R > maxR {
					maxR = p.R
				}
				if p.G > p.R+40 && p.G > maxG {
					maxG = p.G
				}
				if p.B > p.R+40 && p.B > p.G+40 && p.B > maxB {
					maxB = p.B
				}
			}
		}
		if prev != nil && prev.Pix == c.Pix {
			t.Fatalf("frame at %ds is the same as the one before", i)
		}
		prev = c
	}
	if maxR < 120 || maxG < 120 || maxB < 120 {
		t.Errorf("weak colours: red %d, green %d, blue %d", maxR, maxG, maxB)
	}
}

// The treeline is black: the darkest layer, in front of the mountains.
func TestTreeline(t *testing.T) {
	s, _ := New(Options{})
	c := matrix.NewCanvas()
	s.Render(c, 10*time.Second)
	for x := 0; x < matrix.Width; x++ {
		if q := matrix.Quantize(c.Pix[shore-1][x]); q != 0 {
			t.Errorf("shore at column %d is lit: %v", x, c.Pix[shore-1][x])
		}
	}
}

// The mountains are a dark silhouette, but every pixel of them is lit on
// the panel (none go off and vanish into the sky), and the skyline is
// brighter than the slopes below it.
func TestMountainsShowOnPanel(t *testing.T) {
	s, _ := New(Options{})
	c := matrix.NewCanvas()
	s.Render(c, 10*time.Second)
	level := func(p color.RGBA) uint16 {
		q := matrix.Quantize(p)
		return max(q>>8&15, q>>4&15, q&15)
	}
	for x := 0; x < matrix.Width; x++ {
		for y := ridge[x]; y < shore-1; y++ {
			if onPine(x, y) {
				continue
			}
			if l := level(c.Pix[y][x]); l == 0 || (y > ridge[x]+2 && l > 3) {
				t.Errorf("mountain at (%d,%d) is %v: level %d, want a dim silhouette the panel lights", x, y, c.Pix[y][x], l)
			}
		}
		if y := ridge[x]; !onPine(x, y) && level(c.Pix[y][x]) <= level(c.Pix[shore-3][x]) && !onPine(x, shore-3) {
			t.Errorf("skyline at column %d isn't brighter than the slope below", x)
		}
	}
}

func onPine(x, y int) bool {
	for _, p := range pines {
		if y >= shore-p[1] && abs(x-p[0]) <= p[1]/2 {
			return true
		}
	}
	return false
}
