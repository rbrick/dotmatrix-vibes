package minecraft

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"time"

	"dotmatrix/matrix"
)

// Icon values for Options.Icon.
const (
	IconGrass  = "grass"  // a grass block
	IconServer = "server" // the server's own icon, or grass if it has none
)

// Options configures a Scene.
type Options struct {
	Address  string        // "host", "host:port" or an IP
	Icon     string        // IconGrass (default) or IconServer
	Interval time.Duration // how often to ping while shown; default 5s, at least MinInterval
}

// Ping intervals.
const (
	DefaultInterval = 5 * time.Second
	MinInterval     = 2 * time.Second
)

// Scene shows a server's address, a grass block (or the server's icon), its
// latency and how many players are online:
//
//	mc.hypixel.n…   (scrolls when too wide)
//	 ▟██▙   190MS
//	 ████   18721
//	 ▜██▛  OF 200K
//
// It pings the server again every Interval (5 seconds by default) while it
// is shown.
type Scene struct {
	Options

	mu      sync.Mutex
	status  *Status
	err     error
	checked time.Time // when the last ping finished
	pinging bool
	favicon *image.RGBA // the server's icon, scaled to fit
	ping    func(ctx context.Context, addr string) (*Status, error)
}

// New checks the options; it doesn't contact the server.
func New(opts Options) (*Scene, error) {
	opts.Address = strings.TrimSpace(opts.Address)
	if opts.Address == "" {
		return nil, fmt.Errorf("minecraft: no server address")
	}
	switch opts.Icon {
	case "":
		opts.Icon = IconGrass
	case IconGrass, IconServer:
	default:
		return nil, fmt.Errorf("minecraft: icon must be %q or %q", IconGrass, IconServer)
	}
	switch {
	case opts.Interval == 0:
		opts.Interval = DefaultInterval
	case opts.Interval < MinInterval:
		return nil, fmt.Errorf("minecraft: interval must be at least %v", MinInterval)
	}
	return &Scene{Options: opts, ping: Ping}, nil
}

// Refresh pings the server now and waits for the answer.
func (s *Scene) Refresh(ctx context.Context) (*Status, error) {
	st, err := s.ping(ctx, s.Address)
	var icon *image.RGBA
	if err == nil {
		icon = decodeFavicon(st.Favicon)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status, s.err, s.checked = st, err, time.Now()
	if err == nil {
		s.favicon = icon
	}
	return st, err
}

// Status returns the result of the last ping: nil and nil before the first.
func (s *Scene) Status() (*Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status, s.err
}

// refreshIfDue starts a background ping when the last one is too old.
func (s *Scene) refreshIfDue() {
	s.mu.Lock()
	due := !s.pinging && (s.checked.IsZero() || time.Since(s.checked) >= s.Interval)
	if due {
		s.pinging = true
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
		s.pinging = false
		s.mu.Unlock()
	}()
}

var (
	addressColor = color.RGBA{255, 255, 255, 255}
	countColor   = color.RGBA{255, 255, 255, 255}
	onlineColor  = color.RGBA{85, 255, 85, 255} // Minecraft's §a green
	offlineColor = color.RGBA{255, 85, 85, 255} // §c red
	dimColor     = color.RGBA{150, 150, 150, 255}
	grass        = grassBlock()
)

const (
	scrollSpeed = 24 // pixels per second
	iconX       = 2
	iconY       = 9
	panelLeft   = 26 // the status column is centred in x 26-63
)

// Render implements matrix.Scene.
func (s *Scene) Render(c *matrix.Canvas, elapsed time.Duration) time.Duration {
	s.refreshIfDue()
	s.mu.Lock()
	st, err, icon := s.status, s.err, s.favicon
	s.mu.Unlock()

	c.Clear()

	// Address along the top, scrolling if it doesn't fit.
	step := time.Second / scrollSpeed
	w := matrix.TextWidth(s.Address)
	next := time.Second / 2 // show new ping results promptly
	if w > matrix.Width {
		travel := matrix.Width + w
		c.DrawText(matrix.Width-int(elapsed/step)%travel, 0, s.Address, addressColor)
		next = step - elapsed%step
	} else {
		c.DrawText((matrix.Width-w)/2, 0, s.Address, addressColor)
	}

	if s.Icon != IconServer || icon == nil {
		icon = grass
	}
	b := icon.Bounds()
	drawImage(c, iconX+(blockSize-b.Dx())/2, iconY+(blockSize-b.Dy())/2, icon)

	center := func(y int, text string, col color.Color, small bool) {
		if small {
			c.DrawSmallText(panelLeft+(matrix.Width-panelLeft-matrix.SmallTextWidth(text))/2, y, text, col)
		} else {
			c.DrawText(panelLeft+(matrix.Width-panelLeft-matrix.TextWidth(text))/2, y, text, col)
		}
	}
	switch {
	case err != nil:
		center(9, "OFFLINE", offlineColor, true)
		center(16, "-", dimColor, false)
	case st == nil:
		center(9, "PINGING", dimColor, true)
		center(16, "...", dimColor, false)
	default:
		state := "ONLINE"
		if st.Latency > 0 {
			state = fmt.Sprintf("%dMS", st.Latency.Milliseconds())
		}
		center(9, state, onlineColor, true)
		center(16, compact(st.Players.Online, 5), countColor, false)
		center(25, "OF "+compact(st.Players.Max, 6), dimColor, true)
	}
	return next
}

// compact formats n in at most width characters: 18721, 187K, 18.7K, 2.1M.
func compact(n, width int) string {
	if s := fmt.Sprint(n); len(s) <= width {
		return s
	}
	for _, u := range []struct {
		div    float64
		suffix string
	}{{1e3, "K"}, {1e6, "M"}, {1e9, "B"}} {
		v := float64(n) / u.div
		for _, f := range []string{"%.1f", "%.0f"} {
			if s := fmt.Sprintf(f, v) + u.suffix; len(s) <= width && v < 999.5 {
				return strings.Replace(s, ".0"+u.suffix, u.suffix, 1)
			}
		}
	}
	return fmt.Sprint(n)
}

// drawImage draws img with its top-left at (x, y), blending its alpha onto
// the canvas.
func drawImage(c *matrix.Canvas, x, y int, img *image.RGBA) {
	b := img.Bounds()
	for iy := b.Min.Y; iy < b.Max.Y; iy++ {
		for ix := b.Min.X; ix < b.Max.X; ix++ {
			if p := img.RGBAAt(ix, iy); p.A > 0 {
				c.Set(x+ix-b.Min.X, y+iy-b.Min.Y, p) // premultiplied: blends onto black
			}
		}
	}
}

// decodeFavicon decodes a "data:image/png;base64,..." server icon (64x64)
// and shrinks it to the grass block's size by averaging.
func decodeFavicon(uri string) *image.RGBA {
	data, ok := strings.CutPrefix(uri, "data:image/png;base64,")
	if !ok {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(data, "\n", ""))
	if err != nil {
		return nil
	}
	src, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	sb := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, blockSize, blockSize))
	for y := 0; y < blockSize; y++ {
		for x := 0; x < blockSize; x++ {
			x0, x1 := sb.Min.X+x*sb.Dx()/blockSize, sb.Min.X+(x+1)*sb.Dx()/blockSize
			y0, y1 := sb.Min.Y+y*sb.Dy()/blockSize, sb.Min.Y+(y+1)*sb.Dy()/blockSize
			var r, g, bl, a, n uint32
			for sy := y0; sy < max(y1, y0+1); sy++ {
				for sx := x0; sx < max(x1, x0+1); sx++ {
					pr, pg, pb, pa := src.At(sx, sy).RGBA() // premultiplied, 16-bit
					r, g, bl, a, n = r+pr, g+pg, bl+pb, a+pa, n+1
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(bl / n >> 8), uint8(a / n >> 8)})
		}
	}
	return dst
}
