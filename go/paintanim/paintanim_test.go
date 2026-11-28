package paintanim

import (
	"image/color"
	"testing"
	"time"

	"dotmatrix/matrix"
)

// picture is a test picture: a dark blue sea under a light sky with a red sun.
func picture() *matrix.Canvas {
	c := matrix.NewCanvas()
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			col := color.RGBA{230, 210, 170, 255}
			if y > 18+x/16 {
				col = color.RGBA{20, 50, 120, 255}
			}
			if (x-44)*(x-44)+(y-8)*(y-8) <= 16 {
				col = color.RGBA{230, 40, 20, 255}
			}
			c.Pix[y][x] = col
		}
	}
	return c
}

func at(s *Scene, seconds float64) (*matrix.Canvas, time.Duration) {
	c := matrix.NewCanvas()
	next := s.Render(c, time.Duration(seconds*float64(time.Second)))
	return c, next
}

// diff is the mean absolute difference per channel between two canvases.
func diff(a, b *matrix.Canvas) float64 {
	var sum float64
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			p, q := a.Pix[y][x], b.Pix[y][x]
			for _, d := range []int{int(p.R) - int(q.R), int(p.G) - int(q.G), int(p.B) - int(q.B)} {
				sum += float64(max(d, -d))
			}
		}
	}
	return sum / (W * H * 3)
}

func TestEndsExactlyOnThePicture(t *testing.T) {
	target := picture()
	s := New(target, Options{Seconds: 10})
	c, next := at(s, 10)
	if *c != *target {
		t.Errorf("finished painting differs from the picture by %.2f", diff(c, target))
	}
	if next >= 0 {
		t.Error("a finished painting that doesn't loop should be static")
	}
	// Played frame by frame, it ends on the picture too.
	s = New(target, Options{Seconds: 10})
	for ms := 0; ms < 10000; ms += 50 {
		at(s, float64(ms)/1000)
	}
	if c, _ := at(s, 9.999); diff(c, target) > 3 {
		t.Errorf("just before the end, %.2f off", diff(c, target))
	}
}

func TestGetsCloserAsItGoes(t *testing.T) {
	target := picture()
	s := New(target, Options{Seconds: 20})
	start, _ := at(s, 0)
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			if start.Pix[y][x] != (color.RGBA{0, 0, 0, 255}) {
				t.Fatalf("painting should start on a bare (black) canvas; %d,%d is %v", x, y, start.Pix[y][x])
			}
		}
	}
	ground, _ := at(s, 20*groundEnd+0.01)
	g := ground.Pix[16][32]
	if g == (color.RGBA{0, 0, 0, 255}) || g != ground.Pix[0][0] || g != ground.Pix[31][63] {
		t.Errorf("after the ground the canvas should be one toned colour, got %v %v %v", g, ground.Pix[0][0], ground.Pix[31][63])
	}
	// Each layer paints towards a blurred copy of the picture, so the error
	// can rise a little within the refining layers, but it falls overall.
	first := diff(ground, target)
	prev := first
	for _, sec := range []float64{6, 10, 14, 18, 19.9} {
		c, next := at(s, sec)
		if next != frameTime {
			t.Errorf("at %gs next frame in %v, want %v", sec, next, frameTime)
		}
		d := diff(c, target)
		if d > prev+3 {
			t.Errorf("at %gs the canvas is much further from the picture (%.1f) than before (%.1f)", sec, d, prev)
		}
		prev = d
	}
	if prev > first/4 {
		t.Errorf("near the end still %.1f off (the ground was %.1f off)", prev, first)
	}
}

func TestRewindAndLoop(t *testing.T) {
	target := picture()
	s := New(target, Options{Seconds: 10, Loop: true, Hold: 5})
	a, _ := at(s, 4)
	at(s, 8)
	b, _ := at(s, 4) // going back in time starts the round again
	if *a != *b {
		t.Error("rendering the same moment twice should give the same frame")
	}
	held, next := at(s, 12)
	if *held != *target || next <= 0 || next > 3*time.Second+time.Millisecond {
		t.Errorf("during the hold: matches=%v, next=%v", *held == *target, next)
	}
	// The second round paints again, with different strokes.
	second, _ := at(s, 15+4)
	if *second == *a {
		t.Error("a loop should repaint with new strokes")
	}
	if diff(second, matrix.NewCanvas()) == 0 {
		t.Error("the second round should be painting")
	}
}

func TestPlainPicture(t *testing.T) {
	// A one-colour picture needs only the ground, and still finishes.
	c := matrix.NewCanvas()
	c.Fill(color.RGBA{90, 90, 90, 255})
	s := New(c, Options{Seconds: 5})
	if got, _ := at(s, 5); *got != *c {
		t.Error("plain picture should finish exactly")
	}
	if got, _ := at(s, 2); diff(got, c) > 1 {
		t.Error("after the ground a plain picture is already done")
	}
}
