package aurora

import (
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

func TestSilhouetteIsDark(t *testing.T) {
	s, _ := New(Options{})
	c := matrix.NewCanvas()
	s.Render(c, 10*time.Second)
	for x := 0; x < matrix.Width; x++ {
		if p := c.Pix[shore-1][x]; p.R+p.G+p.B > 30 {
			t.Errorf("shore at column %d is lit: %v", x, p)
		}
	}
}
