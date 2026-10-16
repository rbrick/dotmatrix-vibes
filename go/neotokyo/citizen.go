package neotokyo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"dotmatrix/matrix"
)

// DefaultCitizenURL is the citgen server: GET /s{season}/{w}x{h}/{id}
// returns a PNG of that Citizen, or 400 for an ID that doesn't exist.
const DefaultCitizenURL = "https://citgen.rcw.io"

// Highest Citizen IDs to try when picking at random. IDs have gaps (the
// server answers 400 for those), so these are a little above the highest
// found by sampling every 100th ID: S1 had IDs up to 3900, S2 up to 5200.
var maxID = map[int]int{1: 4200, 2: 5500}

// CitizenSource downloads Citizen images and caches them on disk.
type CitizenSource struct {
	BaseURL  string // default DefaultCitizenURL
	CacheDir string // default ~/.cache/matrixd/citizens; "" after defaults disables caching
}

// DefaultCitizenSource fetches from DefaultCitizenURL and caches under the
// user cache directory.
func DefaultCitizenSource() *CitizenSource {
	s := &CitizenSource{BaseURL: DefaultCitizenURL}
	if dir, err := os.UserCacheDir(); err == nil {
		s.CacheDir = filepath.Join(dir, "matrixd", "citizens")
	}
	return s
}

// imageSize is the size downloaded: twice the panel height, so the face can
// be shown at full detail or the whole picture averaged down.
const imageSize = 64

// ErrNoCitizen means the season has no Citizen with that ID.
var ErrNoCitizen = errors.New("no such citizen")

// Image returns the 64x64 picture of a Citizen.
func (s *CitizenSource) Image(ctx context.Context, season, id int) (image.Image, error) {
	cacheFile := ""
	if s.CacheDir != "" {
		cacheFile = filepath.Join(s.CacheDir, fmt.Sprintf("s%d", season), fmt.Sprintf("%d.png", id))
		if data, err := os.ReadFile(cacheFile); err == nil {
			if img, err := png.Decode(bytes.NewReader(data)); err == nil {
				return img, nil
			}
		}
	}
	url := fmt.Sprintf("%s/s%d/%dx%d/%d", s.BaseURL, season, imageSize, imageSize, id)
	data, err := get(ctx, url, "image/png")
	var se *statusError
	if errors.As(err, &se) && se.Code == http.StatusBadRequest {
		return nil, fmt.Errorf("citizen S%d #%d: %w (%s)", season, id, ErrNoCitizen, se.Body)
	}
	if err != nil {
		return nil, fmt.Errorf("citizen S%d #%d: %w", season, id, err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("citizen S%d #%d: %w", season, id, err)
	}
	if cacheFile != "" && os.MkdirAll(filepath.Dir(cacheFile), 0o755) == nil {
		tmp := cacheFile + ".tmp"
		if os.WriteFile(tmp, data, 0o644) == nil {
			os.Rename(tmp, cacheFile) // a failed cache write only costs a re-download
		}
	}
	return img, nil
}

// CitizenOptions configures a CitizenScene.
type CitizenOptions struct {
	Season int           // 1 or 2; 0 with ID 0 picks either season at random
	ID     int           // a Citizen number, or 0 for a different random Citizen every Dwell
	Zoom   bool          // show the face at full detail instead of the whole picture
	Label  bool          // show "CITIZEN #1234 SEASON 1" beside the picture
	Dwell  time.Duration // time per Citizen when random; default 20s
}

// CitizenScene shows a Neo Tokyo Citizen.
type CitizenScene struct {
	CitizenOptions
	src *CitizenSource

	mu       sync.Mutex
	cur      citizen
	next     *citizen // prefetched when random
	fetching bool
	since    time.Duration // elapsed when cur appeared
}

type citizen struct {
	season, id int
	img        image.Image // 64x64
}

// NewCitizen checks the options and downloads the first Citizen, so a bad
// ID or a network problem is reported straight away.
func NewCitizen(ctx context.Context, src *CitizenSource, opts CitizenOptions) (*CitizenScene, error) {
	if opts.ID < 0 {
		return nil, fmt.Errorf("citizen: bad ID %d", opts.ID)
	}
	if opts.Season == 0 && opts.ID != 0 {
		opts.Season = 1
	}
	if _, ok := maxID[opts.Season]; !ok && opts.Season != 0 {
		return nil, fmt.Errorf("citizen: season must be 1 or 2")
	}
	if opts.Dwell <= 0 {
		opts.Dwell = 20 * time.Second
	}
	s := &CitizenScene{CitizenOptions: opts, src: src}
	var err error
	if opts.ID != 0 {
		s.cur.season, s.cur.id = opts.Season, opts.ID
		s.cur.img, err = src.Image(ctx, opts.Season, opts.ID)
	} else {
		s.cur, err = s.random(ctx)
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Current returns the season and ID of the Citizen on screen.
func (s *CitizenScene) Current() (season, id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur.season, s.cur.id
}

// random downloads a random Citizen, trying other IDs where there is none.
// Missing IDs are answered quickly, so a few dozen tries take seconds.
func (s *CitizenScene) random(ctx context.Context) (citizen, error) {
	var last error
	for range 40 {
		season := s.Season
		if season == 0 {
			season = 1 + rand.IntN(2)
		}
		id := 1 + rand.IntN(maxID[season])
		img, err := s.src.Image(ctx, season, id)
		if err == nil {
			return citizen{season, id, img}, nil
		}
		if !errors.Is(err, ErrNoCitizen) {
			return citizen{}, err
		}
		last = err
	}
	return citizen{}, fmt.Errorf("citizen: no random Citizen found: %w", last)
}

// prefetch loads the next random Citizen in the background; s.mu is held.
func (s *CitizenScene) prefetch() {
	s.fetching = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		next, err := s.random(ctx)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.fetching = false
		if err == nil {
			s.next = &next
		}
		// On error the next Render tries again.
	}()
}

// Render implements matrix.Scene: the picture on the left (or centred
// without a label), and its name on the right.
func (s *CitizenScene) Render(c *matrix.Canvas, elapsed time.Duration) time.Duration {
	s.mu.Lock()
	if s.ID == 0 {
		if elapsed-s.since >= s.Dwell && s.next != nil {
			s.cur, s.next, s.since = *s.next, nil, elapsed
		}
		if s.next == nil && !s.fetching {
			s.prefetch()
		}
	}
	cur := s.cur
	s.mu.Unlock()

	c.Clear()
	x0 := 0
	if !s.Label {
		x0 = (matrix.Width - matrix.Height) / 2
	}
	drawPortrait(c, x0, cur.img, s.Zoom)
	if s.Label {
		const left = matrix.Height + 1 // the right half, after a gap
		center := func(w int) int { return left + (matrix.Width-left-w)/2 }
		c.DrawSmallText(center(matrix.SmallTextWidth("CITIZEN")), 3, "CITIZEN", pink)
		num := fmt.Sprintf("#%d", cur.id)
		c.DrawText(center(matrix.TextWidth(num)), 12, num, white)
		season := fmt.Sprintf("SEASON %d", cur.season)
		c.DrawSmallText(center(matrix.SmallTextWidth(season)), 24, season, cyan)
	}
	return time.Second
}

// drawPortrait draws a 64x64 Citizen picture in a 32x32 square at x0:
// zoomed, the middle of its top half (where the face is) pixel for pixel;
// otherwise the whole picture at half size, averaging each 2x2 block.
func drawPortrait(c *matrix.Canvas, x0 int, img image.Image, zoom bool) {
	if img == nil {
		return
	}
	b := img.Bounds()
	const n = matrix.Height
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if zoom {
				c.Set(x0+x, y, img.At(b.Min.X+n/2+x, b.Min.Y+2+y))
				continue
			}
			var r, g, bl uint32
			for _, d := range [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
				pr, pg, pb, _ := img.At(b.Min.X+2*x+d[0], b.Min.Y+2*y+d[1]).RGBA()
				r, g, bl = r+pr, g+pg, bl+pb
			}
			c.Set(x0+x, y, color.RGBA64{uint16(r / 4), uint16(g / 4), uint16(bl / 4), 0xffff})
		}
	}
}
