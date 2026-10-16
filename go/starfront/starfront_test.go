package starfront

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dotmatrix/matrix"
)

var now = time.Date(2026, 10, 14, 16, 4, 0, 0, time.UTC)

// roofs makes n fresh readings, open where open[i] is true.
func roofs(open ...bool) []Roof {
	rs := make([]Roof, len(open))
	for i, o := range open {
		rs[i] = Roof{Device: i + 1, Name: fmt.Sprintf("Building %d", i+1), Open: o, Updated: now.Add(-time.Minute)}
	}
	return rs
}

func TestFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"id":"x","device_number":2,"name":"Building 2","is_open":true,"state_update":"2026-10-14T16:03:05.442000Z"},`+
			`{"id":"y","device_number":1,"name":"Building 1","is_open":false,"state_update":"2026-10-14T16:03:54.712000Z"}]`)
	}))
	defer srv.Close()
	rs, err := Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || rs[0].Device != 1 || rs[0].Open || !rs[1].Open || rs[1].Name != "Building 2" ||
		!rs[0].Updated.Equal(time.Date(2026, 10, 14, 16, 3, 54, 712e6, time.UTC)) {
		t.Errorf("roofs = %+v", rs)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer bad.Close()
	if _, err := Fetch(context.Background(), bad.URL); err == nil {
		t.Error("HTTP error accepted")
	}
}

func TestConsensus(t *testing.T) {
	f, tr := false, true
	stale := roofs(tr, tr, f)
	stale[0].Updated, stale[1].Updated = now.Add(-time.Hour), now.Add(-time.Hour)
	for _, c := range []struct {
		roofs   []Roof
		want    state
		summary string
	}{
		// One roof reporting open among closed ones doesn't open the observatory.
		{roofs(tr, f, f, f, f, f, f, f, f, f, f, f, f, f), closed, "1 OF 14 OPEN"},
		{roofs(f, tr, tr, tr, tr), open, "4 OF 5 OPEN"},
		{roofs(f, f, f), closed, "ALL 3 CLOSED"},
		{roofs(tr, tr), open, "ALL 2 OPEN"},
		{roofs(tr, f, tr, f), mixed, "2 OF 4 OPEN"},
		{stale, closed, "ALL 1 CLOSED"}, // stale readings don't vote
		{nil, unknown, "NO RECENT DATA"},
	} {
		st, summary := summarize(c.roofs, now)
		if st != c.want || summary != c.summary {
			t.Errorf("%v: got %v %q, want %v %q", c.roofs, st, summary, c.want, c.summary)
		}
	}
}

func TestNewValidates(t *testing.T) {
	if _, err := New(Options{Interval: time.Second}); err == nil {
		t.Error("interval under the minimum accepted")
	}
	if _, err := New(Options{Building: -1}); err == nil {
		t.Error("negative building accepted")
	}
	if s, _ := New(Options{}); s.Interval != DefaultInterval || s.URL != DefaultURL {
		t.Errorf("defaults: %+v", s.Options)
	}
}

func TestRender(t *testing.T) {
	s, _ := New(Options{})
	s.now = func() time.Time { return now }
	var fail bool
	data := roofs(true, false, false, false)
	s.fetch = func(context.Context, string) ([]Roof, error) {
		if fail {
			return nil, errors.New("down")
		}
		return data, nil
	}
	c := matrix.NewCanvas()
	s.Refresh(context.Background())
	s.Render(c, 0)
	// Headline CLOSED in red; the misreporting building's square is green.
	if !has(c, closedColor, 9, 16) || has(c, openColor, 9, 16) || !has(c, openColor, 26, 29) {
		t.Error("expected red CLOSED with one green square")
	}

	// Following building 1: its own reading.
	s.Building = 1
	s.Render(c, 0)
	if !has(c, openColor, 9, 16) || !has(c, markerColor, 30, 31) {
		t.Error("building 1 should show OPEN with a marker")
	}

	// A failed check keeps the last reading.
	s.Building = 0
	fail = true
	if _, err := s.Refresh(context.Background()); err == nil {
		t.Fatal("expected failure")
	}
	if rs, err := s.Roofs(); len(rs) != 4 || err == nil {
		t.Errorf("roofs %v, err %v", rs, err)
	}
	s.Render(c, 0)
	if !has(c, closedColor, 9, 16) {
		t.Error("last reading should still show")
	}
	if !strings.Contains(fmt.Sprint(s.err), "down") {
		t.Error(s.err)
	}
}

// has reports whether col appears in rows y0 to y1.
func has(c *matrix.Canvas, col color.RGBA, y0, y1 int) bool {
	for y := y0; y <= y1; y++ {
		for x := range c.Pix[y] {
			if c.Pix[y][x] == col {
				return true
			}
		}
	}
	return false
}
