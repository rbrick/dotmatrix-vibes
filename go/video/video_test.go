package video

import (
	"bytes"
	"context"
	"errors"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dotmatrix/matrix"
)

// frames returns n raw frames; frame i is filled with grey level 10*(i+1).
func frames(n int) []byte {
	var b []byte
	for i := 0; i < n; i++ {
		b = append(b, bytes.Repeat([]byte{byte(10 * (i + 1))}, frameBytes)...)
	}
	return b
}

// fakeSource plays the given bytes, counting how many times it was started.
func fakeSource(data []byte, starts *atomic.Int32) source {
	return func(ctx context.Context, o Options) (io.Reader, func() error, error) {
		starts.Add(1)
		return bytes.NewReader(data), func() error { return nil }, nil
	}
}

func newScene(t *testing.T, o Options, src source) *Scene {
	t.Helper()
	if o.URL == "" {
		o.URL = "https://www.youtube.com/watch?v=aqz-KE-bpKQ"
	}
	s, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	s.source = src
	return s
}

// renderUntil renders every 10ms (as the Player would) until check passes.
func renderUntil(t *testing.T, s *Scene, start time.Time, check func(*matrix.Canvas, time.Duration) bool) {
	t.Helper()
	c := matrix.NewCanvas()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if next := s.Render(c, time.Since(start)); check(c, next) {
			return
		}
	}
	t.Fatal("condition never met")
}

func TestNewChecksOptions(t *testing.T) {
	for _, u := range []string{"", "not a url", "ftp://example.com/v.mp4", "file:///etc/passwd", "https://"} {
		if _, err := New(Options{URL: u}); err == nil {
			t.Errorf("%q should be refused", u)
		}
	}
	if _, err := New(Options{URL: "https://youtu.be/x", Brighten: 5}); err == nil {
		t.Error("brighten 5 should be refused")
	}
	s, err := New(Options{URL: " https://youtu.be/aqz-KE-bpKQ "})
	if err != nil || s.Options().Brighten != 1.3 || s.Options().URL != "https://youtu.be/aqz-KE-bpKQ" {
		t.Errorf("defaults: %+v %v", s.Options(), err)
	}
}

func TestPlaysToTheEnd(t *testing.T) {
	var starts atomic.Int32
	s := newScene(t, Options{}, fakeSource(frames(3), &starts))
	start := time.Now()
	// Before any frame: the LOADING status, still animating.
	c := matrix.NewCanvas()
	s.mu.Lock()
	s.running = true // keep it from starting during this first look
	s.mu.Unlock()
	if next := s.Render(c, 0); next != time.Second/fps {
		t.Errorf("next = %v", next)
	}
	lit := 0
	for y := range c.Pix {
		for x := range c.Pix[y] {
			if c.Pix[y][x] != (color.RGBA{}) {
				lit++
			}
		}
	}
	if lit == 0 {
		t.Error("LOADING should be drawn before the first frame")
	}
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	// It ends on the last frame and stops asking for redraws.
	renderUntil(t, s, start, func(c *matrix.Canvas, next time.Duration) bool {
		return next < 0 && c.Pix[0][0] == color.RGBA{30, 30, 30, 255}
	})
	if starts.Load() != 1 {
		t.Errorf("started %d times", starts.Load())
	}
}

func TestLoopPlaysAgain(t *testing.T) {
	var starts atomic.Int32
	s := newScene(t, Options{Loop: true}, fakeSource(frames(2), &starts))
	start := time.Now()
	renderUntil(t, s, start, func(*matrix.Canvas, time.Duration) bool { return starts.Load() >= 3 })
	s.Stop()
}

func TestErrorBeforeAnyFrameIsShown(t *testing.T) {
	s := newScene(t, Options{Loop: true}, func(ctx context.Context, o Options) (io.Reader, func() error, error) {
		return strings.NewReader(""), func() error { return errors.New("yt-dlp: Video unavailable") }, nil
	})
	renderUntil(t, s, time.Now(), func(c *matrix.Canvas, next time.Duration) bool { return next < 0 })
	s.mu.Lock()
	defer s.mu.Unlock()
	if !strings.Contains(s.status, "Video unavailable") {
		t.Errorf("status = %q", s.status)
	}
}

func TestStopsWhenNoLongerShown(t *testing.T) {
	stopped := make(chan struct{})
	var once sync.Once
	s := newScene(t, Options{}, func(ctx context.Context, o Options) (io.Reader, func() error, error) {
		r, w := io.Pipe()
		go func() {
			frame := frames(1)
			for ctx.Err() == nil {
				w.Write(frame)
				time.Sleep(20 * time.Millisecond)
			}
			w.Close()
			once.Do(func() { close(stopped) })
		}()
		return r, func() error { return nil }, nil
	})
	start := time.Now()
	renderUntil(t, s, start, func(c *matrix.Canvas, _ time.Duration) bool { return c.Pix[0][0] == color.RGBA{10, 10, 10, 255} })
	// The Player moves on and stops rendering it: the pipeline must stop.
	select {
	case <-stopped:
	case <-time.After(idleStop + 2*time.Second):
		t.Fatal("pipeline kept running after the scene stopped being shown")
	}
	// Shown again later, it starts over.
	s.Render(matrix.NewCanvas(), 0)
	s.mu.Lock()
	running := s.running
	s.mu.Unlock()
	if !running {
		t.Error("showing it again should start playback")
	}
	s.Stop()
}

// TestPipeline runs the real yt-dlp | ffmpeg wiring with stand-in scripts.
func TestPipeline(t *testing.T) {
	dir := t.TempDir()
	script := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	oldY, oldF := YTDLP, FFmpeg
	t.Cleanup(func() { YTDLP, FFmpeg = oldY, oldF })

	// yt-dlp writes "video"; ffmpeg turns its input into two red frames.
	YTDLP = script("yt-dlp", `printf video`)
	FFmpeg = script("ffmpeg", `cat >/dev/null; i=0; while [ $i -lt 4096 ]; do printf '\377\000\000'; i=$((i+1)); done`)
	r, wait, err := ytdlpSource(context.Background(), Options{URL: "https://example.com/v", Brighten: 1.3})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r)
	if err := wait(); err != nil {
		t.Fatal(err)
	}
	if len(b) != 2*frameBytes || b[0] != 255 || b[1] != 0 {
		t.Errorf("got %d bytes starting %v", len(b), b[:3])
	}

	// A yt-dlp failure is reported with its last error line.
	YTDLP = script("yt-dlp", `echo "ERROR: [youtube] abc: Video unavailable" >&2; exit 1`)
	FFmpeg = script("ffmpeg", `cat >/dev/null`)
	r, wait, err = ytdlpSource(context.Background(), Options{URL: "https://example.com/v", Brighten: 1.3})
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(r)
	if err := wait(); err == nil || !strings.Contains(err.Error(), "[youtube] abc: Video unavailable") {
		t.Errorf("err = %v", err)
	}

	// Missing commands are reported.
	YTDLP = filepath.Join(dir, "no-such-yt-dlp")
	if _, _, err := ytdlpSource(context.Background(), Options{URL: "https://example.com/v"}); err == nil {
		t.Error("a missing yt-dlp should be an error")
	}
}
