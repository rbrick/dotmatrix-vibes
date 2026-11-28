package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"

	"dotmatrix/matrix"
	"dotmatrix/paintanim"
)

// paintRequest asks for a picture to be shown being painted:
//
//	POST /api/paint {"seconds":30,"loop":false,"image":"<base64 PNG, JPEG or GIF>"}
//
// Without an image it paints what the panel shows now (or, if that is a
// painting still in progress, the finished picture).
type paintRequest struct {
	Seconds float64 `json:"seconds,omitempty"` // painting time, 1-600; default 30
	Loop    bool    `json:"loop,omitempty"`    // repaint (differently) after showing the result for a while
	Image   []byte  `json:"image,omitempty"`   // base64 in JSON
}

// paintingSettings is how a painting is saved: the picture itself is the
// content's Image.
type paintingSettings struct {
	Seconds float64 `json:"seconds"`
	Loop    bool    `json:"loop,omitempty"`
}

func (r paintRequest) options() (paintanim.Options, error) {
	if r.Seconds != 0 && (r.Seconds < 1 || r.Seconds > 600) {
		return paintanim.Options{}, errors.New("seconds must be between 1 and 600")
	}
	return paintanim.Options{Seconds: r.Seconds, Loop: r.Loop}, nil
}

// paintingTarget returns the picture to paint: the request's image scaled
// to fit, or what the player shows now.
func paintingTarget(player *matrix.Player, data []byte) (*matrix.Canvas, error) {
	c := matrix.NewCanvas()
	if len(data) > 0 {
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("decoding image: %w", err)
		}
		c.DrawImage(img)
		return c, nil
	}
	if p, ok := player.State().Scene.(*paintanim.Scene); ok {
		*c = *p.Target()
		return c, nil
	}
	frame, _ := player.Frame()
	*c = frame
	return c, nil
}

func handlePaint(mux *http.ServeMux, player *matrix.Player, persist func()) {
	mux.HandleFunc("POST /api/paint", func(w http.ResponseWriter, r *http.Request) {
		var req paintRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
			http.Error(w, `expected {"seconds":30,"loop":false,"image":"<base64>"} (all optional)`, http.StatusBadRequest)
			return
		}
		opts, err := req.options()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		target, err := paintingTarget(player, req.Image)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		player.Show(paintanim.New(target, opts))
		persist()
		writeState(w, player)
	})
}

// paintingContent describes a painting scene for the state file.
func paintingContent(c *content, p *paintanim.Scene) {
	c.Mode = "painting"
	var buf bytes.Buffer
	png.Encode(&buf, p.Target()) // cannot fail writing to a buffer
	c.Image = buf.Bytes()
	o := p.Options()
	c.Painting = &paintingSettings{Seconds: o.Seconds, Loop: o.Loop}
}

// paintingScene rebuilds a saved painting. It starts painting again.
func (c content) paintingScene() (*paintanim.Scene, error) {
	if c.Painting == nil {
		return nil, errors.New(`mode "painting" without settings`)
	}
	img, err := png.Decode(bytes.NewReader(c.Image))
	if err != nil {
		return nil, fmt.Errorf("saved painting: %w", err)
	}
	target := matrix.NewCanvas()
	target.DrawImage(img)
	return paintanim.New(target, paintanim.Options{Seconds: c.Painting.Seconds, Loop: c.Painting.Loop}), nil
}
