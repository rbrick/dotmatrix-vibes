package pokemon

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Source downloads sprites and caches them on disk.
type Source struct {
	BaseURL  string // root of the PokéSprite repository; default: GitHub
	CacheDir string // default: ~/.cache/matrixd/pokesprite; "" after defaults disables caching
	Client   *http.Client
}

const defaultBaseURL = "https://raw.githubusercontent.com/msikma/pokesprite/master"

// DefaultSource fetches from GitHub and caches under the user cache dir.
func DefaultSource() *Source {
	s := &Source{BaseURL: defaultBaseURL, Client: &http.Client{Timeout: 15 * time.Second}}
	if dir, err := os.UserCacheDir(); err == nil {
		s.CacheDir = filepath.Join(dir, "matrixd", "pokesprite")
	}
	return s
}

// Sprite returns the box sprite for e.
func (s *Source) Sprite(ctx context.Context, e Entry, shiny bool) (image.Image, error) {
	variant := "regular"
	if shiny {
		variant = "shiny"
	}
	img, err := s.image(ctx, "pokemon-gen8/"+variant+"/"+e.Slug+".png", filepath.Join(variant, e.Slug+".png"))
	if err != nil {
		return nil, fmt.Errorf("pokemon: %s sprite: %w", e.Name, err)
	}
	return img, nil
}

// BallSprite returns the item sprite for a Poké Ball.
func (s *Source) BallSprite(ctx context.Context, b Ball) (image.Image, error) {
	path, cacheFile := ballPath(b)
	img, err := s.image(ctx, path, cacheFile)
	if err != nil {
		return nil, fmt.Errorf("pokemon: %s sprite: %w", b.Name(), err)
	}
	return img, nil
}

// BallPNG returns a Poké Ball's sprite as a PNG file.
func (s *Source) BallPNG(ctx context.Context, b Ball) ([]byte, error) {
	path, cacheFile := ballPath(b)
	return s.file(ctx, path, cacheFile)
}

func ballPath(b Ball) (path, cacheFile string) {
	return "items/ball/" + b.Slug + ".png", filepath.Join("ball", b.Slug+".png")
}

// file returns the PNG file at path in the repository, from the cache file
// if there is one, otherwise downloading and caching it.
func (s *Source) file(ctx context.Context, path, cacheFile string) ([]byte, error) {
	var data []byte
	if s.CacheDir != "" {
		data, _ = os.ReadFile(filepath.Join(s.CacheDir, cacheFile))
	}
	if data != nil {
		return data, nil
	}
	data, err := s.download(ctx, path)
	if err != nil {
		return nil, err
	}
	if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
		return nil, err // don't cache something that isn't a PNG
	}
	if s.CacheDir != "" {
		s.store(cacheFile, data) // a failed cache write only costs a re-download
	}
	return data, nil
}

func (s *Source) image(ctx context.Context, path, cacheFile string) (image.Image, error) {
	data, err := s.file(ctx, path, cacheFile)
	if err != nil {
		return nil, err
	}
	return png.Decode(bytes.NewReader(data))
}

func (s *Source) download(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.BaseURL+"/"+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

func (s *Source) store(file string, data []byte) {
	path := filepath.Join(s.CacheDir, file)
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		os.Rename(tmp, path)
	}
}
