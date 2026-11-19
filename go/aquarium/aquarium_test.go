package aquarium

import (
	"testing"
	"time"

	"dotmatrix/matrix"
)

func run(s *Scene, seconds float64) *matrix.Canvas {
	c := matrix.NewCanvas()
	for f := 0; f <= int(seconds*25); f++ {
		if next := s.Render(c, time.Duration(f)*frameInterval); next != frameInterval {
			panic("unexpected frame interval")
		}
	}
	return c
}

// Over ten minutes every fish keeps moving, stays in the water and the free
// swimmers visit both ends of the tank.
func TestFishSwim(t *testing.T) {
	s := New()
	c := matrix.NewCanvas()
	type span struct{ minX, maxX float64 }
	spans := make([]span, len(s.fish))
	for i := range spans {
		spans[i] = span{1e9, -1e9}
	}
	still := make([]int, len(s.fish))
	const frames = 25 * 600
	for f := 0; f <= frames; f++ {
		s.Render(c, time.Duration(f)*frameInterval)
		for i, fi := range s.fish {
			spans[i].minX = min(spans[i].minX, fi.x)
			spans[i].maxX = max(spans[i].maxX, fi.x)
			if fi.vx*fi.vx+fi.vy*fi.vy < 0.04 {
				still[i]++
			}
			_, h := fi.size()
			if fi.y-h/2 < 0 || fi.y+h/2 > sandTop+1 {
				t.Fatalf("fish %d left the water at y=%.1f", i, fi.y)
			}
		}
	}
	for i, fi := range s.fish {
		if still[i] > frames/10 {
			t.Errorf("fish %d was still %d%% of the time", i, still[i]*100/frames)
		}
		if fi.kind.home == nil && (spans[i].minX > 20 || spans[i].maxX < 44) {
			t.Errorf("fish %d only swam between x=%.0f and %.0f", i, spans[i].minX, spans[i].maxX)
		}
	}
}

// Each species shows up in its colours: clownfish orange, tang royal blue
// with a yellow tail, betta crimson.
func TestSpeciesVisible(t *testing.T) {
	c := run(New(), 3)
	found := map[[3]uint8]bool{}
	for _, row := range c.Pix {
		for _, p := range row {
			found[[3]uint8{p.R, p.G, p.B}] = true
		}
	}
	for name, col := range map[string][3]uint8{
		"clownfish": {orange.R, orange.G, orange.B},
		"blue tang": {royal.R, royal.G, royal.B},
		"tang tail": {yellow.R, yellow.G, yellow.B},
		"betta":     {crimson.R, crimson.G, crimson.B},
	} {
		if !found[col] {
			t.Errorf("no %s on screen", name)
		}
	}
}

// Showing the scene again (elapsed starting over) doesn't make the fish
// jump.
func TestRestartDoesNotJump(t *testing.T) {
	s := New()
	run(s, 5)
	x := s.fish[2].x
	s.Render(matrix.NewCanvas(), 0)
	if s.fish[2].x != x {
		t.Errorf("fish moved from %.1f to %.1f when elapsed restarted", x, s.fish[2].x)
	}
}
