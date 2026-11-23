package fractal

import (
	"testing"
	"time"

	"dotmatrix/matrix"
)

func TestNew(t *testing.T) {
	s, err := New(Options{})
	if err != nil || s.Set != Both || s.Speed != 1 {
		t.Fatalf("New(defaults) = %+v, %v", s, err)
	}
	for _, o := range []Options{{Set: "sierpinski"}, {Speed: -1}, {Speed: 11}} {
		if _, err := New(o); err == nil {
			t.Errorf("New(%+v) succeeded", o)
		}
	}
}

// colourful counts the distinct colours on the canvas and how many pixels
// are black.
func colourful(c *matrix.Canvas) (colours, black int) {
	seen := map[[3]uint8]bool{}
	for _, row := range c.Pix {
		for _, p := range row {
			seen[[3]uint8{p.R, p.G, p.B}] = true
			if p.R == 0 && p.G == 0 && p.B == 0 {
				black++
			}
		}
	}
	return len(seen), black
}

// Every dive, at its deepest, and every stage of the Julia loop should be
// busy with colour rather than flat or mostly black.
func TestFramesAreColourful(t *testing.T) {
	check := func(name string, set string, at float64) {
		s, _ := New(Options{Set: set})
		c := matrix.NewCanvas()
		if next := s.Render(c, time.Duration(at*float64(time.Second))); next != frameInterval {
			t.Errorf("%s: next frame in %v, want %v", name, next, frameInterval)
		}
		colours, black := colourful(c)
		if colours < 100 || black > matrix.Width*matrix.Height/2 {
			t.Errorf("%s at %gs: %d colours, %d black pixels", name, at, colours, black)
		}
	}
	for i := range mandelbrot.targets {
		check("mandelbrot dive", Mandelbrot, (float64(i)+0.5)*diveSeconds)
	}
	for i := range ship.targets {
		for _, phase := range []float64{0.25, 0.5} {
			check("burning ship dive", BurningShip, (float64(i)+phase)*diveSeconds)
		}
	}
	for i := 0; i < 8; i++ {
		check("julia", Julia, float64(i)*juliaSeconds/8)
	}
}

// A dive starts and ends at the same zoomed-out view, so moving on to the
// next target doesn't jump.
func TestDivesJoinUp(t *testing.T) {
	for _, set := range []string{Mandelbrot, BurningShip} {
		s, _ := New(Options{Set: set})
		end, start := matrix.NewCanvas(), matrix.NewCanvas()
		s.Render(end, time.Duration((diveSeconds-0.001)*float64(time.Second)))
		s.Render(start, time.Duration(diveSeconds*float64(time.Second)))
		// Only the palette changes, so the set's shape (its black pixels)
		// should match, give or take an edge pixel or two blended differently.
		_, b1 := colourful(end)
		_, b2 := colourful(start)
		if d := b1 - b2; d < -5 || d > 5 {
			t.Errorf("%s: set shape jumps between dives: %d vs %d black pixels", set, b1, b2)
		}
	}
}

// The Burning Ship is its own picture, not the Mandelbrot set.
func TestBurningShip(t *testing.T) {
	if _, err := New(Options{Set: BurningShip}); err != nil {
		t.Fatal(err)
	}
	// -0.8-0.6i is in the ship's hull but outside the Mandelbrot set.
	if _, escaped := escape(-0.8, -0.6, -0.8, -0.6, 500, true); escaped {
		t.Error("-0.8-0.6i escaped the Burning Ship")
	}
	if _, escaped := escape(-0.8, -0.6, -0.8, -0.6, 500, false); !escaped {
		t.Error("-0.8-0.6i didn't escape the Mandelbrot set")
	}
	// i lies in the Mandelbrot set but escapes the Burning Ship.
	if _, escaped := escape(0, 1, 0, 1, 500, false); escaped {
		t.Error("i escaped the Mandelbrot set")
	}
	if _, escaped := escape(0, 1, 0, 1, 500, true); !escaped {
		t.Error("i didn't escape the Burning Ship")
	}
}

func TestBothTakesTurns(t *testing.T) {
	s, _ := New(Options{})
	m, _ := New(Options{Set: Mandelbrot})
	j, _ := New(Options{Set: Julia})
	same := func(a, b *Scene, ta, tb float64) bool {
		ca, cb := matrix.NewCanvas(), matrix.NewCanvas()
		a.Render(ca, time.Duration(ta*float64(time.Second)))
		b.Render(cb, time.Duration(tb*float64(time.Second)))
		return ca.Pix == cb.Pix
	}
	if !same(s, m, 10, 10) {
		t.Error("both: first turn isn't the Mandelbrot dive")
	}
	if !same(s, j, diveSeconds+10, 10) {
		t.Error("both: second turn isn't the Julia loop")
	}
	if !same(s, m, diveSeconds+juliaSeconds+10, diveSeconds+10) {
		t.Error("both: third turn isn't the next Mandelbrot dive")
	}
}
