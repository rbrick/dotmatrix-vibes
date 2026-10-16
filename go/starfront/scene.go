package starfront

import (
	"context"
	"fmt"
	"image/color"
	"sync"
	"time"

	"dotmatrix/matrix"
)

// Options configures a Scene.
type Options struct {
	Building int           // a device number to follow, or 0 for the whole observatory
	Interval time.Duration // how often to check while shown; default 15s, at least MinInterval
	URL      string        // default DefaultURL
}

// Check intervals.
const (
	DefaultInterval = 15 * time.Second
	MinInterval     = 5 * time.Second
)

// Scene shows whether Starfront's roofs are open. With no Building the
// headline is the consensus: OPEN or CLOSED when most buildings with a
// recent reading agree, so one roof misreporting doesn't flip it, and MIXED
// on a tie; the line under it counts the open roofs. With a Building it
// shows that building's own reading. Either way a row of squares along the bottom shows every
// building: green open, red closed, grey unknown.
type Scene struct {
	Options

	mu       sync.Mutex
	roofs    []Roof
	err      error
	checked  time.Time
	fetching bool
	fetch    func(ctx context.Context, url string) ([]Roof, error)
	now      func() time.Time
}

// New checks the options; it doesn't contact the API.
func New(opts Options) (*Scene, error) {
	if opts.Building < 0 {
		return nil, fmt.Errorf("starfront: bad building %d", opts.Building)
	}
	switch {
	case opts.Interval == 0:
		opts.Interval = DefaultInterval
	case opts.Interval < MinInterval:
		return nil, fmt.Errorf("starfront: interval must be at least %v", MinInterval)
	}
	if opts.URL == "" {
		opts.URL = DefaultURL
	}
	return &Scene{Options: opts, fetch: Fetch, now: time.Now}, nil
}

// Refresh reads the roof state now and waits for the answer. After an
// error the last good reading is kept.
func (s *Scene) Refresh(ctx context.Context) ([]Roof, error) {
	roofs, err := s.fetch(ctx, s.URL)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err, s.checked = err, time.Now()
	if err == nil {
		s.roofs = roofs
	}
	return roofs, err
}

// Roofs returns the last good reading and the last error, if the latest
// check failed.
func (s *Scene) Roofs() ([]Roof, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.roofs, s.err
}

func (s *Scene) refreshIfDue() {
	s.mu.Lock()
	due := !s.fetching && (s.checked.IsZero() || time.Since(s.checked) >= s.Interval)
	if due {
		s.fetching = true
	}
	s.mu.Unlock()
	if !due {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.Refresh(ctx)
		s.mu.Lock()
		s.fetching = false
		s.mu.Unlock()
	}()
}

// state is what the scene says about the observatory or one building.
type state int

const (
	unknown state = iota
	closed
	open
	mixed // as many open as closed
)

var (
	titleColor   = color.RGBA{140, 170, 255, 255}
	openColor    = color.RGBA{60, 255, 60, 255}
	closedColor  = color.RGBA{255, 50, 40, 255}
	unknownColor = color.RGBA{120, 120, 120, 255}
	summaryColor = color.RGBA{200, 200, 200, 255}
	markerColor  = color.RGBA{255, 255, 255, 255}
	mixedColor   = color.RGBA{255, 170, 0, 255}
	dimClosed    = color.RGBA{110, 20, 15, 255}
)

func (st state) color() color.RGBA {
	switch st {
	case open:
		return openColor
	case closed:
		return closedColor
	case mixed:
		return mixedColor
	}
	return unknownColor
}

func roofState(r Roof, now time.Time) state {
	switch {
	case r.Stale(now):
		return unknown
	case r.Open:
		return open
	}
	return closed
}

// Render implements matrix.Scene.
func (s *Scene) Render(c *matrix.Canvas, _ time.Duration) time.Duration {
	s.refreshIfDue()
	s.mu.Lock()
	roofs, err, checked := s.roofs, s.err, s.checked
	s.mu.Unlock()
	now := s.now()

	c.Clear()
	center := func(y int, text string, col color.Color) {
		c.DrawSmallText((matrix.Width-matrix.SmallTextWidth(text))/2, y, text, col)
	}
	title := "STARFRONT"
	c.DrawText((matrix.Width-matrix.TextWidth(title))/2, 0, title, titleColor)

	// The headline and the line under it.
	st, word, summary := unknown, "...", "CHECKING"
	switch {
	case roofs == nil && err != nil:
		word, summary = "NO DATA", "NO CONNECTION"
	case roofs == nil && checked.IsZero():
	case s.Building != 0:
		word, summary = "UNKNOWN", fmt.Sprintf("NO BUILDING %d", s.Building)
		for _, r := range roofs {
			if r.Device == s.Building {
				st, summary = roofState(r, now), fmt.Sprintf("BUILDING %d", r.Device)
			}
		}
	default:
		st, summary = summarize(roofs, now)
	}
	if st != unknown {
		word = map[state]string{open: "OPEN", closed: "CLOSED", mixed: "MIXED"}[st]
	}
	if err != nil && roofs != nil {
		summary = "NO CONNECTION" // showing the last good reading
	}

	headline := ":telescope:" + word
	c.DrawText((matrix.Width-matrix.TextWidth(headline))/2, 9, headline, st.color()) // the emoji keeps its colours
	center(19, summary, summaryColor)

	// One square per building along the bottom.
	size := 3
	for size > 1 && len(roofs)*(size+1)-1 > matrix.Width {
		size--
	}
	x := (matrix.Width - (len(roofs)*(size+1) - 1)) / 2
	for _, r := range roofs {
		col := roofState(r, now).color()
		if col == closedColor && s.Building != 0 && r.Device != s.Building {
			col = dimClosed
		}
		for dy := 0; dy < size; dy++ {
			for dx := 0; dx < size; dx++ {
				c.Set(x+dx, 26+dy, col)
			}
		}
		if r.Device == s.Building {
			for dx := 0; dx < size; dx++ {
				c.Set(x+dx, 27+size, markerColor)
			}
		}
		x += size + 1
	}
	return time.Second / 2 // show new readings promptly
}

// summarize gives the consensus of the buildings with recent readings
// (a strict majority, or mixed on a tie) and counts the open ones.
func summarize(roofs []Roof, now time.Time) (state, string) {
	var nOpen, nClosed int
	for _, r := range roofs {
		switch roofState(r, now) {
		case open:
			nOpen++
		case closed:
			nClosed++
		}
	}
	known := nOpen + nClosed
	st := mixed
	switch {
	case known == 0:
		return unknown, "NO RECENT DATA"
	case nOpen*2 > known:
		st = open
	case nClosed*2 > known:
		st = closed
	}
	switch {
	case nOpen == known:
		return st, fmt.Sprintf("ALL %d OPEN", known)
	case nClosed == known:
		return st, fmt.Sprintf("ALL %d CLOSED", known)
	}
	return st, fmt.Sprintf("%d OF %d OPEN", nOpen, known)
}
