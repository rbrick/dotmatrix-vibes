package main

import (
	"image/color"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dotmatrix/matrix"
	"dotmatrix/video"
)

// TestVideoEndpoint plays a "video" through stand-in yt-dlp and ffmpeg
// commands and checks it reaches the panel, then that it stops when another
// display takes over.
func TestVideoEndpoint(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	oldY, oldF := video.YTDLP, video.FFmpeg
	t.Cleanup(func() { video.YTDLP, video.FFmpeg = oldY, oldF })
	video.YTDLP = write("yt-dlp", `printf video`)
	// ffmpeg: one green frame every 50ms until killed, recording its pid.
	video.FFmpeg = write("ffmpeg", `echo $$ > `+filepath.Join(dir, "ffmpeg.pid")+`
cat >/dev/null
while :; do i=0; while [ $i -lt 2048 ]; do printf '\000\377\000\000\377\000'; i=$((i+1)); done; sleep 0.05; done`)

	p := newPlayer(t)
	mux := http.NewServeMux()
	registerVideo(mux, p, func() {})

	post := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/video", strings.NewReader(body)))
		return w
	}
	for _, bad := range []string{`{"url":"file:///etc/passwd"}`, `{"url":""}`, `{`} {
		if w := post(bad); w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d", bad, w.Code)
		}
	}
	if w := post(`{"url":"https://www.youtube.com/watch?v=aqz-KE-bpKQ"}`); w.Code != 200 {
		t.Fatalf("play: %d %s", w.Code, w.Body)
	}
	waitFrame(t, p, func(c *matrix.Canvas) bool { return c.Pix[16][32] == color.RGBA{0, 255, 0, 255} })
	if s := snapshot(p); s.Mode != "video" || s.Video == nil || s.Video.URL == "" {
		t.Errorf("snapshot = %+v", s.sceneSpec)
	}
	if st := p.State().Scene.(*video.Scene).Status(); st != "playing" {
		t.Errorf("status = %q", st)
	}

	// Another display takes over: ffmpeg must be gone a few seconds later.
	p.ShowText(matrix.Text{Text: "HI"})
	pidBytes, _ := os.ReadFile(filepath.Join(dir, "ffmpeg.pid"))
	pid := strings.TrimSpace(string(pidBytes))
	deadline := time.Now().Add(8 * time.Second)
	for {
		// ps -p exits non-zero once the process is gone (Linux and macOS).
		if exec.Command("ps", "-p", pid).Run() != nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("ffmpeg (pid %s) still running after the video stopped being shown", pid)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
