package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"sync"

	"dotmatrix/connect4"
	"dotmatrix/matrix"
)

// content is what matrixd remembers across restarts: what the panel shows
// and how bright it is. It is saved as JSON after every change and is also
// part of the /api/state response.
type content struct {
	Brightness uint8            `json:"brightness"`
	sceneSpec                   // Mode is also "connect4", "rotation" or "image"
	Connect4   *connect4.State  `json:"connect4,omitempty"`
	Rotation   *rotationRequest `json:"rotation,omitempty"`
	Image      []byte           `json:"image,omitempty"` // PNG; state file only
}

// snapshot captures the Player's current content.
func snapshot(p *matrix.Player) content {
	s := p.State()
	c := content{Brightness: s.Brightness}
	if spec, ok := specOf(s.Scene); ok {
		c.sceneSpec = spec
		return c
	}
	switch sc := s.Scene.(type) {
	case *connect4.Game:
		c.Mode = "connect4"
		gs := sc.State()
		c.Connect4 = &gs
	case *rotation:
		c.Mode = "rotation"
		c.Rotation = &sc.req
	case matrix.Still:
		c.Mode = "image"
		var buf bytes.Buffer
		png.Encode(&buf, sc.Canvas) // cannot fail writing to a buffer
		c.Image = buf.Bytes()
	}
	return c
}

// apply shows saved content on the Player. A restored game is passed to
// attach before it is shown.
func (c content) apply(p *matrix.Player, attach func(*connect4.Game)) error {
	switch c.Mode {
	case "connect4":
		if c.Connect4 == nil {
			return errors.New(`mode "connect4" without a game`)
		}
		g, err := connect4.Restore(*c.Connect4)
		if err != nil {
			return err
		}
		attach(g)
		p.Show(g)
	case "rotation":
		if c.Rotation == nil {
			return errors.New(`mode "rotation" without views`)
		}
		r, err := c.Rotation.scene()
		if err != nil {
			return err
		}
		p.Show(r)
	case "image":
		img, err := png.Decode(bytes.NewReader(c.Image))
		if err != nil {
			return fmt.Errorf("saved image: %w", err)
		}
		p.ShowImage(img)
	default:
		s, err := c.build()
		if err != nil {
			return err
		}
		p.Show(s)
	}
	p.SetBrightness(c.Brightness)
	return nil
}

// stateFile saves and loads content. A zero stateFile (no path) does nothing.
type stateFile struct {
	path string
	mu   sync.Mutex // serialises saves from concurrent requests
}

func defaultStatePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "matrixd", "state.json")
}

// load returns the saved content; the error wraps os.ErrNotExist if nothing
// has been saved yet.
func (f *stateFile) load() (content, error) {
	var c content
	if f.path == "" {
		return c, os.ErrNotExist
	}
	b, err := os.ReadFile(f.path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("%s: %w", f.path, err)
	}
	return c, nil
}

// save writes c atomically, so a crash mid-write never leaves a broken file.
func (f *stateFile) save(c content) error {
	if f.path == "" {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.path), ".state-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), f.path)
}
