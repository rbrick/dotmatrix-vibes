package minecraft

import (
	"context"
	"errors"
	"image/color"
	"testing"
	"time"

	"dotmatrix/matrix"
)

func TestCompact(t *testing.T) {
	for n, want := range map[int]string{
		7: "7", 18721: "18721", 200000: "200K", 123456: "123K", 1500000: "1.5M", 999999: "1.0M",
	} {
		if got := compact(n, 5); got != want && !(n == 999999 && got == "1M") {
			t.Errorf("compact(%d, 5) = %q, want %q", n, got, want)
		}
	}
	if got := compact(200000, 6); got != "200000" {
		t.Errorf("compact(200000, 6) = %q", got)
	}
}

func TestNewValidates(t *testing.T) {
	if _, err := New(Options{Address: "  "}); err == nil {
		t.Error("empty address accepted")
	}
	if _, err := New(Options{Address: "a", Icon: "creeper"}); err == nil {
		t.Error("unknown icon accepted")
	}
	if _, err := New(Options{Address: "a", Interval: time.Second}); err == nil {
		t.Error("interval under the minimum accepted")
	}
	if s, _ := New(Options{Address: " mc.example.com "}); s.Address != "mc.example.com" || s.Icon != IconGrass || s.Interval != DefaultInterval {
		t.Errorf("defaults: %+v", s.Options)
	}
}

func TestSceneStates(t *testing.T) {
	s, _ := New(Options{Address: "a-rather-long-server-address.example.com"})
	calls := make(chan struct{}, 10)
	var fail bool
	s.ping = func(ctx context.Context, addr string) (*Status, error) {
		calls <- struct{}{}
		if fail {
			return nil, errors.New("connection refused")
		}
		st := &Status{Latency: 42 * time.Millisecond}
		st.Players.Online, st.Players.Max = 12, 20
		return st, nil
	}
	c := matrix.NewCanvas()

	// First render: nothing known yet, a ping starts in the background, and
	// the long address scrolls.
	if next := s.Render(c, 0); next != time.Second/scrollSpeed {
		t.Errorf("scrolling address redraws in %v", next)
	}
	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("no background ping")
	}
	for i := 0; i < 100; i++ {
		if st, _ := s.Status(); st != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if st, err := s.Status(); st == nil || err != nil {
		t.Fatalf("status %v, %v", st, err)
	}
	s.Render(c, 0)
	if !hasColor(c, onlineColor) {
		t.Error("online server should show green latency")
	}
	// Not due again until Interval has passed.
	s.Render(c, time.Second)
	select {
	case <-calls:
		t.Error("pinged again too soon")
	case <-time.After(50 * time.Millisecond):
	}

	fail = true
	if _, err := s.Refresh(context.Background()); err == nil {
		t.Fatal("expected failure")
	}
	s.Render(c, 0)
	if !hasColor(c, offlineColor) || hasColor(c, onlineColor) {
		t.Error("unreachable server should show OFFLINE in red")
	}
}

func hasColor(c *matrix.Canvas, col color.RGBA) bool {
	for y := range c.Pix {
		for x := range c.Pix[y] {
			if c.Pix[y][x] == col {
				return true
			}
		}
	}
	return false
}
