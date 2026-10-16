package planets

import (
	"testing"
	"time"

	"dotmatrix/matrix"
)

func TestNewValidates(t *testing.T) {
	if _, err := New(Options{Body: "vulcan"}); err == nil {
		t.Error("unknown body should be rejected")
	}
	s, err := New(Options{Body: "Saturn"})
	if err != nil || s.Body != "saturn" {
		t.Errorf("New(Saturn) = %v, %v", s, err)
	}
	if s, _ := New(Options{}); s.Body != Tour || s.Speed != 1 || s.Dwell != 12*time.Second {
		t.Errorf("defaults not applied: %+v", s.Options)
	}
}

func TestEveryBodyRendersSomething(t *testing.T) {
	for _, name := range Names() {
		s, _ := New(Options{Body: name})
		c := matrix.NewCanvas()
		if next := s.Render(c, 3*time.Second); next != frameInterval {
			t.Errorf("%s: next frame in %v", name, next)
		}
		// The centre of the panel is on the body and should be lit.
		if p := c.Pix[matrix.Height/2][matrix.Width/2-4]; matrix.Quantize(p) == 0 {
			t.Errorf("%s: centre pixel is black", name)
		}
	}
}

func TestTourCyclesAndFades(t *testing.T) {
	s, _ := New(Options{Dwell: 10 * time.Second})
	if b, _ := s.current(5); b.name != "sun" {
		t.Errorf("t=5s shows %s, want sun", b.name)
	}
	if b, _ := s.current(15); b.name != "mercury" {
		t.Errorf("t=15s shows %s, want mercury", b.name)
	}
	if b, _ := s.current(float64(10 * len(bodies))); b.name != "sun" {
		t.Errorf("tour should wrap around to the sun, got %s", b.name)
	}
	if _, fade := s.current(10.1); fade > 0.5 {
		t.Errorf("should be fading in just after a switch, fade = %v", fade)
	}
	if _, fade := s.current(15); fade != 1 {
		t.Errorf("mid-dwell fade = %v, want 1", fade)
	}
}

func TestRenderIsFastEnough(t *testing.T) {
	s, _ := New(Options{Body: "saturn", Label: true})
	c := matrix.NewCanvas()
	s.Render(c, 0) // bake the texture first
	start := time.Now()
	const n = 30
	for i := 0; i < n; i++ {
		s.Render(c, time.Duration(i)*frameInterval)
	}
	if per := time.Since(start) / n; per > 15*time.Millisecond {
		t.Errorf("rendering takes %v per frame; too slow for 30 fps", per)
	}
}
