package pokemon

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"dotmatrix/matrix"
)

// Random is the Options.Pokemon value that shows a different random
// Pokémon every Dwell.
const Random = "random"

// Options configures a Scene.
type Options struct {
	Pokemon string        // name or Pokédex number, or Random
	Shiny   bool          // use the shiny colouring
	Label   bool          // show the name and number
	Dwell   time.Duration // time per Pokémon in Random mode, default 10s
}

// Scene implements matrix.Scene.
type Scene struct {
	Options
	src *Source

	mu       sync.Mutex
	cur      shown
	next     *shown // prefetched in Random mode
	fetching bool
	since    float64 // elapsed seconds when cur appeared
}

type shown struct {
	entry  Entry
	shiny  bool
	sprite *image.RGBA // cropped to the creature and scaled to fit
}

// New resolves opts.Pokemon and downloads its sprite (or a random one), so
// a bad name or a network problem is reported straight away.
func New(ctx context.Context, src *Source, opts Options) (*Scene, error) {
	if opts.Dwell <= 0 {
		opts.Dwell = 10 * time.Second
	}
	s := &Scene{Options: opts, src: src}
	var e Entry
	if s.random() {
		e = Pokedex[rand.IntN(len(Pokedex))]
	} else {
		var err error
		if e, err = Lookup(opts.Pokemon); err != nil {
			return nil, err
		}
		s.Pokemon = e.Name
	}
	cur, err := s.load(ctx, e)
	if err != nil {
		return nil, err
	}
	s.cur = cur
	return s, nil
}

func (s *Scene) random() bool { return strings.EqualFold(s.Pokemon, Random) }

// Current returns the Pokémon on screen.
func (s *Scene) Current() Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur.entry
}

func (s *Scene) load(ctx context.Context, e Entry) (shown, error) {
	img, err := s.src.Sprite(ctx, e, s.Shiny)
	if err != nil {
		return shown{}, err
	}
	return shown{entry: e, shiny: s.Shiny, sprite: fit(crop(img))}, nil
}

// prefetch loads the next random Pokémon in the background; s.mu is held.
func (s *Scene) prefetch() {
	s.fetching = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		next, err := s.load(ctx, Pokedex[rand.IntN(len(Pokedex))])
		s.mu.Lock()
		defer s.mu.Unlock()
		s.fetching = false
		if err == nil {
			s.next = &next
		}
		// On error the next Render tries again with another Pokémon.
	}()
}

var (
	labelColor = color.RGBA{220, 220, 220, 255}
	dexColor   = color.RGBA{130, 130, 130, 255}
	black      = color.RGBA{0, 0, 0, 255}
	sparkle    = color.RGBA{255, 230, 80, 255}
)

const hopPeriod = 0.5 // seconds per hop up or down, like icons in the games

// Render implements matrix.Scene.
func (s *Scene) Render(c *matrix.Canvas, elapsed time.Duration) time.Duration {
	t := elapsed.Seconds()
	s.mu.Lock()
	if s.random() {
		if t-s.since >= s.Dwell.Seconds() && s.next != nil {
			s.cur, s.next, s.since = *s.next, nil, t
		}
		if s.next == nil && !s.fetching {
			s.prefetch()
		}
	}
	cur := s.cur
	s.mu.Unlock()

	c.Clear()
	b := cur.sprite.Bounds()
	x0 := (matrix.Width - b.Dx()) / 2
	y0 := (matrix.Height - b.Dy()) / 2
	if int(t/hopPeriod)%2 == 1 {
		y0-- // hop
	}
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			p := cur.sprite.RGBAAt(x, y)
			if p.A > 0 {
				c.Set(x0+x, y0+y, p) // premultiplied, so this blends onto black
			}
		}
	}

	if s.Label {
		name, dex := cur.entry.Label(), fmt.Sprintf("%03d", cur.entry.Dex) // "#" is unreadable at 3x5
		dexX, dexY := matrix.Width-1-matrix.SmallTextWidth(dex), 1
		if 1+matrix.SmallTextWidth(name)+3 > dexX {
			dexY = matrix.Height - 1 - matrix.SmallTextHeight // long name: number goes bottom right
		}
		outlined(c, []label{{1, 1, name, labelColor}, {dexX, dexY, dex, dexColor}})
	}
	if cur.shiny {
		for _, d := range [][2]int{{0, 0}, {-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
			c.Set(2+d[0], matrix.Height-3+d[1], sparkle)
		}
	}
	// Redraw at the next hop.
	next := hopPeriod - mod(t, hopPeriod)
	return time.Duration(next * float64(time.Second))
}

func mod(a, b float64) float64 { return a - b*float64(int(a/b)) }

type label struct {
	x, y int
	text string
	col  color.Color
}

// outlined draws small text with a black outline so it stays readable on
// top of the sprite. All outlines go down first so that one label's outline
// never cuts into another label.
func outlined(c *matrix.Canvas, labels []label) {
	for _, l := range labels {
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				c.DrawSmallText(l.x+dx, l.y+dy, l.text, black)
			}
		}
	}
	for _, l := range labels {
		c.DrawSmallText(l.x, l.y, l.text, l.col)
	}
}

// crop trims the transparent margin around the creature; box sprites are
// 68x56 with the creature somewhere inside.
func crop(img image.Image) *image.RGBA {
	b := img.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X-1, b.Min.Y-1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x), max(maxY, y)
			}
		}
	}
	if maxX < minX {
		return image.NewRGBA(image.Rect(0, 0, 1, 1)) // fully transparent
	}
	out := image.NewRGBA(image.Rect(0, 0, maxX-minX+1, maxY-minY+1))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			out.Set(x-minX, y-minY, img.At(x, y))
		}
	}
	return out
}

// fit shrinks a sprite that is larger than the panel, averaging the source
// pixels under each output pixel. Sprites that fit are left pixel-perfect.
func fit(src *image.RGBA) *image.RGBA {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	scale := min(1, float64(matrix.Width)/float64(w), float64(matrix.Height)/float64(h))
	if scale == 1 {
		return src
	}
	dw, dh := max(1, int(float64(w)*scale+0.5)), max(1, int(float64(h)*scale+0.5))
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	const n = 4 // samples per axis per output pixel
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var r, g, b, a int
			for j := 0; j < n; j++ {
				for i := 0; i < n; i++ {
					sx := int((float64(x) + (float64(i)+0.5)/n) / scale)
					sy := int((float64(y) + (float64(j)+0.5)/n) / scale)
					p := src.RGBAAt(min(sx, w-1), min(sy, h-1))
					r, g, b, a = r+int(p.R), g+int(p.G), b+int(p.B), a+int(p.A)
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r / (n * n)), uint8(g / (n * n)), uint8(b / (n * n)), uint8(a / (n * n))})
		}
	}
	return dst
}
