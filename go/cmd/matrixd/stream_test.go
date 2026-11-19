package main

import (
	"bufio"
	"encoding/base64"
	"image"
	"image/color"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dotmatrix/matrix"
)

// The stream sends the panel's picture even with no panel connected.
func TestFrameStream(t *testing.T) {
	player := newPlayer(t)
	img := image.NewRGBA(image.Rect(0, 0, matrix.Width, matrix.Height))
	img.Set(3, 4, color.RGBA{255, 10, 20, 255})
	player.ShowImage(img)
	player.SetBrightness(77)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { streamFrames(w, r, player) }))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}
	lines := bufio.NewScanner(resp.Body)
	lines.Buffer(nil, 1<<16)
	deadline := time.Now().Add(5 * time.Second)
	for lines.Scan() && time.Now().Before(deadline) {
		data, ok := strings.CutPrefix(lines.Text(), "data: ")
		if !ok {
			continue
		}
		bright, enc, _ := strings.Cut(data, ",")
		rgb, err := base64.StdEncoding.DecodeString(enc)
		if err != nil || len(rgb) != matrix.Width*matrix.Height*3 {
			t.Fatalf("frame: %d bytes, %v", len(rgb), err)
		}
		// The pixel as the panel gets it: 16 levels a channel.
		q := matrix.Quantize(color.RGBA{255, 10, 20, 255})
		want := [3]byte{byte(q>>8&15) * 17, byte(q>>4&15) * 17, byte(q&15) * 17}
		i := (4*matrix.Width + 3) * 3
		if bright == "77" && [3]byte(rgb[i:i+3]) == want {
			return // the picture shown, at the brightness set
		}
	}
	t.Fatal("never saw the frame shown")
}
