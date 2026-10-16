package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"dotmatrix/connect4"
	"dotmatrix/matrix"
)

// content is what matrixd remembers across restarts: what the panel shows
// and how bright it is. It is saved as JSON after every change and is also
// part of the /api/state response.
type content struct {
	Brightness uint8            `json:"brightness,omitempty"`
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

// savedScene is content saved under a name to show again later. Its
// Brightness is unused: showing a scene keeps the panel's brightness.
type savedScene struct {
	Name string `json:"name"`
	content
}

// stateData is the state file: the current content plus the saved scenes.
type stateData struct {
	content
	Scenes []savedScene `json:"scenes,omitempty"`
}

// stateFile saves and loads content and the saved scenes. A stateFile with
// no path keeps scenes in memory only.
type stateFile struct {
	path   string
	mu     sync.Mutex // serialises saves and guards scenes
	scenes []savedScene
}

func defaultStatePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "matrixd", "state.json")
}

// load returns the saved content and keeps the saved scenes; the error wraps
// os.ErrNotExist if nothing has been saved yet.
func (f *stateFile) load() (content, error) {
	if f.path == "" {
		return content{}, os.ErrNotExist
	}
	b, err := os.ReadFile(f.path)
	if err != nil {
		return content{}, err
	}
	var d stateData
	if err := json.Unmarshal(b, &d); err != nil {
		return content{}, fmt.Errorf("%s: %w", f.path, err)
	}
	f.mu.Lock()
	f.scenes = d.Scenes
	f.mu.Unlock()
	return d.content, nil
}

const maxScenes = 100

// sceneList returns a copy of the saved scenes, in the order they were first
// saved.
func (f *stateFile) sceneList() []savedScene {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]savedScene{}, f.scenes...)
}

// scene returns the saved scene called name.
func (f *stateFile) scene(name string) (savedScene, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.scenes {
		if s.Name == name {
			return s, true
		}
	}
	return savedScene{}, false
}

// putScene saves s, replacing any scene with the same name, and writes the
// file with cur as the current content.
func (f *stateFile) putScene(s savedScene, cur content) error {
	s.Brightness = 0
	f.mu.Lock()
	defer f.mu.Unlock()
	i := slices.IndexFunc(f.scenes, func(o savedScene) bool { return o.Name == s.Name })
	switch {
	case i >= 0:
		f.scenes[i] = s
	case len(f.scenes) >= maxScenes:
		return fmt.Errorf("at most %d saved scenes; delete one first", maxScenes)
	default:
		f.scenes = append(f.scenes, s)
	}
	return f.write(cur)
}

// deleteScene removes the scene called name, reporting whether there was
// one, and writes the file with cur as the current content.
func (f *stateFile) deleteScene(name string, cur content) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := len(f.scenes)
	f.scenes = slices.DeleteFunc(f.scenes, func(s savedScene) bool { return s.Name == name })
	if len(f.scenes) == n {
		return false, nil
	}
	return true, f.write(cur)
}

// save writes c and the saved scenes.
func (f *stateFile) save(c content) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.write(c)
}

// write saves c and the scenes atomically, so a crash mid-write never leaves
// a broken file. f.mu must be held.
func (f *stateFile) write(c content) error {
	if f.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(stateData{c, f.scenes}, "", "  ")
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
