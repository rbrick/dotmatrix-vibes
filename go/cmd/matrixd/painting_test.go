package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dotmatrix/matrix"
	"dotmatrix/paintanim"
)

func postPaint(t *testing.T, mux *http.ServeMux, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/paint", bytes.NewReader(b)))
	return w
}

func TestPaintEndpoint(t *testing.T) {
	p := newPlayer(t)
	saves := 0
	mux := http.NewServeMux()
	handlePaint(mux, p, func() { saves++ })
	red := color.RGBA{255, 0, 0, 255}

	// With an image: paints that image.
	img := image.NewRGBA(image.Rect(0, 0, matrix.Width, matrix.Height))
	img.Set(5, 6, red)
	var buf bytes.Buffer
	png.Encode(&buf, img)
	if w := postPaint(t, mux, map[string]any{"seconds": 12, "image": buf.Bytes()}); w.Code != 200 {
		t.Fatalf("paint with image: %d %s", w.Code, w.Body)
	}
	sc, ok := p.State().Scene.(*paintanim.Scene)
	if !ok {
		t.Fatalf("scene is %T, want a painting", p.State().Scene)
	}
	if sc.Target().Pix[6][5] != red || sc.Options().Seconds != 12 {
		t.Errorf("painting target or seconds wrong: %v %v", sc.Target().Pix[6][5], sc.Options().Seconds)
	}
	if got := snapshot(p).Mode; got != "painting" || saves != 1 {
		t.Errorf("mode %q after %d saves", got, saves)
	}

	// Without an image while a painting is in progress: repaints its
	// finished picture, not the half-painted frame.
	if w := postPaint(t, mux, map[string]any{"loop": true}); w.Code != 200 {
		t.Fatalf("repaint: %d %s", w.Code, w.Body)
	}
	again := p.State().Scene.(*paintanim.Scene)
	if again == sc || *again.Target() != *sc.Target() || !again.Options().Loop {
		t.Error("repainting a painting should paint its finished picture again, looping")
	}

	// Without an image over a still picture: paints what's showing.
	still := matrix.NewCanvas()
	still.Fill(color.RGBA{0, 0, 255, 255})
	p.Show(matrix.Still{Canvas: still})
	waitFrame(t, p, func(c *matrix.Canvas) bool { return c.Pix[0][0] == color.RGBA{0, 0, 255, 255} })
	postPaint(t, mux, map[string]any{})
	if got := p.State().Scene.(*paintanim.Scene).Target().Pix[0][0]; got != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("painted %v, want what was showing", got)
	}

	for _, bad := range []any{map[string]any{"seconds": 0.5}, map[string]any{"seconds": 601}, map[string]any{"image": []byte("not a picture")}} {
		if w := postPaint(t, mux, bad); w.Code != http.StatusBadRequest {
			t.Errorf("%v: status %d, want 400", bad, w.Code)
		}
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/paint", strings.NewReader("{")))
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad JSON: status %d", w.Code)
	}
}

func TestMCPDrawAnimate(t *testing.T) {
	cs, p, _ := connectMCP(t)
	res := call(t, cs, "draw", map[string]any{"base": "blank", "animate": 15,
		"ops": []map[string]any{{"op": "clear", "color": "green"}}})
	if res.IsError {
		t.Fatalf("draw failed: %v", res.Content)
	}
	sc, ok := p.State().Scene.(*paintanim.Scene)
	if !ok || sc.Options().Seconds != 15 || sc.Target().Pix[0][0] != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("scene %T should paint the green picture over 15s", p.State().Scene)
	}
	if res := call(t, cs, "draw", map[string]any{"animate": 0.2, "ops": []map[string]any{}}); !res.IsError {
		t.Error("animate below 1 second should fail")
	}
}
