// Package video plays a video (a YouTube link, or anything else yt-dlp or
// ffmpeg can open) on the panel. yt-dlp fetches a low-resolution stream,
// ffmpeg decodes it in real time and scales it to 64×32, and the Scene shows
// each frame as it arrives. There is no sound.
//
// The pipeline starts when the Scene is first drawn and stops by itself a
// few seconds after the Scene stops being drawn (another display took its
// place), so nothing keeps downloading in the background. Shown again, it
// starts from the beginning.
package video

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/color"
	"io"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dotmatrix/matrix"
)

// Options say what to play and how.
type Options struct {
	URL      string  // a YouTube (or other yt-dlp supported) link, or a direct video URL
	Fit      bool    // show the whole frame with black bars, instead of filling the panel (cropping)
	Loop     bool    // play again from the start when it ends
	Brighten float64 // gamma lift for the LEDs, which show dark tones as black: 1 is as is; default 1.3
}

const (
	fps        = 20
	frameBytes = matrix.Width * matrix.Height * 3
	// idleStop is how long after the last Render the pipeline is stopped.
	idleStop = 3 * time.Second
)

// Commands, settable for tests or unusual installs.
var (
	YTDLP  = "yt-dlp"
	FFmpeg = "ffmpeg"
)

// source starts the decoder and returns its raw RGB frames, and a function
// that waits for it to finish and reports why it failed.
type source func(ctx context.Context, o Options) (frames io.Reader, wait func() error, err error)

// Scene plays a video. It implements matrix.Scene.
type Scene struct {
	opts   Options
	source source

	mu         sync.Mutex
	frame      matrix.Canvas
	haveFrame  bool
	status     string // shown before the first frame: "LOADING", an error…
	running    bool
	ended      bool // played to the end without looping
	cancel     context.CancelFunc
	lastRender time.Time
	lastElapse time.Duration
}

// New checks o and returns a Scene that plays it.
func New(o Options) (*Scene, error) {
	u, err := url.Parse(strings.TrimSpace(o.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("video: %q is not an http(s) link", o.URL)
	}
	o.URL = u.String()
	if o.Brighten == 0 {
		o.Brighten = 1.3
	}
	if o.Brighten < 0.5 || o.Brighten > 3 {
		return nil, errors.New("video: brighten must be between 0.5 and 3")
	}
	return &Scene{opts: o, source: ytdlpSource, status: "LOADING"}, nil
}

// Options returns the scene's options, with defaults filled in.
func (s *Scene) Options() Options { return s.opts }

// Render implements matrix.Scene.
func (s *Scene) Render(c *matrix.Canvas, elapsed time.Duration) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	// Shown again (elapsed went back) after the pipeline was stopped, or
	// still running from an earlier showing: start over.
	if elapsed < s.lastElapse && s.running {
		s.stopLocked()
	}
	if elapsed < s.lastElapse {
		s.ended, s.haveFrame, s.status = false, false, "LOADING"
	}
	s.lastElapse = elapsed
	s.lastRender = now
	if !s.running && !s.ended {
		s.startLocked()
	}
	if s.haveFrame {
		*c = s.frame
	} else {
		drawStatus(c, s.status)
	}
	if s.ended {
		return -1
	}
	return time.Second / fps
}

// Status says how playback is going: "loading", "playing", "ended",
// "stopped" or the error that stopped it.
func (s *Scene) Status() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case strings.HasPrefix(s.status, "VIDEO ERROR: "):
		return "error: " + strings.TrimPrefix(s.status, "VIDEO ERROR: ")
	case s.ended:
		return "ended"
	case !s.running:
		return "stopped"
	case s.haveFrame:
		return "playing"
	}
	return "loading"
}

// Stop ends playback now. Rendering again starts it over.
func (s *Scene) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
}

func (s *Scene) stopLocked() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.running = false
}

func (s *Scene) startLocked() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.running = cancel, true
	go s.play(ctx)
	go s.watchdog(ctx, cancel)
}

// watchdog stops playback once the Scene is no longer being drawn.
func (s *Scene) watchdog(ctx context.Context, cancel context.CancelFunc) {
	t := time.NewTicker(idleStop / 3)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.mu.Lock()
			idle := time.Since(s.lastRender) > idleStop
			if idle && s.cancel != nil {
				s.stopLocked()
			}
			s.mu.Unlock()
			if idle {
				cancel()
				return
			}
		}
	}
}

// play runs the pipeline until the video ends, fails or ctx is cancelled.
func (s *Scene) play(ctx context.Context) {
	for {
		frames, wait, err := s.source(ctx, s.opts)
		got := 0
		if err == nil {
			buf := make([]byte, frameBytes)
			for {
				if _, err = io.ReadFull(frames, buf); err != nil {
					break
				}
				got++
				s.mu.Lock()
				if ctx.Err() != nil { // stopped: don't draw over a newer run
					s.mu.Unlock()
					break
				}
				for i := 0; i < matrix.Width*matrix.Height; i++ {
					s.frame.Pix[i/matrix.Width][i%matrix.Width] = color.RGBA{buf[3*i], buf[3*i+1], buf[3*i+2], 255}
				}
				s.haveFrame = true
				s.mu.Unlock()
			}
			if werr := wait(); werr != nil && ctx.Err() == nil {
				err = werr
			} else if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				err = nil
			}
		}
		if ctx.Err() != nil {
			return // stopped: no longer shown, or shown again
		}
		s.mu.Lock()
		if err != nil && got == 0 {
			s.status = "VIDEO ERROR: " + err.Error()
			s.haveFrame = false
		}
		again := s.opts.Loop && got > 0
		if !again {
			s.ended, s.running = true, false
			s.cancel = nil
		}
		s.mu.Unlock()
		if !again {
			return
		}
	}
}

// ytdlpSource pipes yt-dlp's download into ffmpeg, which plays it in real
// time (-re) as 64×32 RGB frames.
func ytdlpSource(ctx context.Context, o Options) (io.Reader, func() error, error) {
	ytdlp, ffmpeg := YTDLP, FFmpeg // read once: the pipeline outlives this call
	dl := exec.CommandContext(ctx, ytdlp, "--quiet", "--no-warnings", "--no-playlist",
		// The panel is 64×32: the smallest H.264 video-only stream is plenty.
		"-f", "bv*[height<=240][vcodec^=avc1]/bv*[height<=360]/b[height<=360]/w",
		"-o", "-", o.URL)
	scale := "scale=64:32:force_original_aspect_ratio=increase:flags=area,crop=64:32"
	if o.Fit {
		scale = "scale=64:32:force_original_aspect_ratio=decrease:flags=area,pad=64:32:(ow-iw)/2:(oh-ih)/2"
	}
	ff := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-re", "-i", "pipe:0", "-an",
		"-vf", fmt.Sprintf("fps=%d,%s,eq=gamma=%.2f:saturation=1.15", fps, scale, o.Brighten),
		"-pix_fmt", "rgb24", "-f", "rawvideo", "pipe:1")
	var dlErr, ffErr tail
	dl.Stderr, ff.Stderr = &dlErr, &ffErr
	pipe, err := dl.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	ff.Stdin = pipe
	out, err := ff.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := dl.Start(); err != nil {
		return nil, nil, fmt.Errorf("starting %s: %w", ytdlp, err)
	}
	if err := ff.Start(); err != nil {
		dl.Process.Kill()
		dl.Wait()
		return nil, nil, fmt.Errorf("starting %s: %w", ffmpeg, err)
	}
	// ffmpeg has its own copy of the pipe; dropping ours means yt-dlp gets
	// a broken pipe instead of blocking if ffmpeg exits early.
	pipe.Close()
	wait := func() error {
		ffWait := ff.Wait()
		dlWait := dl.Wait()
		switch {
		case dlWait != nil:
			return fmt.Errorf("%s: %s", filepath.Base(ytdlp), dlErr.last(dlWait))
		case ffWait != nil:
			return fmt.Errorf("%s: %s", filepath.Base(ffmpeg), ffErr.last(ffWait))
		}
		return nil
	}
	return out, wait, nil
}

// tail keeps the end of a command's error output.
type tail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > 2000 {
		t.buf = t.buf[len(t.buf)-2000:]
	}
	return len(p), nil
}

// last returns the last non-empty line written, or err.
func (t *tail) last(err error) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	lines := bytes.Split(bytes.TrimSpace(t.buf), []byte("\n"))
	if l := strings.TrimSpace(string(lines[len(lines)-1])); l != "" {
		return strings.TrimPrefix(l, "ERROR: ")
	}
	return err.Error()
}

// drawStatus shows a short status before the first frame: LOADING, or an
// error, small and dim so it doesn't look like part of the video.
func drawStatus(c *matrix.Canvas, status string) {
	c.Clear()
	col := color.RGBA{120, 120, 120, 255}
	if strings.HasPrefix(status, "VIDEO ERROR") {
		col = color.RGBA{200, 60, 40, 255}
	}
	// Up to five lines of 15 characters in the small font.
	words := strings.Fields(strings.ToUpper(status))
	var lines []string
	line := ""
	for _, w := range words {
		if len(line)+len(w)+1 > 15 && line != "" {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += w
	}
	lines = append(lines, line)
	if len(lines) > 5 {
		lines = lines[:5]
	}
	y := (matrix.Height - len(lines)*6 + 1) / 2
	for _, l := range lines {
		c.DrawSmallText((matrix.Width-matrix.SmallTextWidth(l))/2, y, l, col)
		y += 6
	}
}
