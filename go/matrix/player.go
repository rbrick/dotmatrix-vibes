package matrix

import (
	"image"
	"image/color"
	"strings"
	"sync"
	"time"
)

// Scroll controls whether text moves across the panel.
type Scroll string

const (
	ScrollAuto   Scroll = "auto"   // scroll only if a line is wider than the panel
	ScrollAlways Scroll = "always" // always scroll
	ScrollNever  Scroll = "never"  // never scroll; long lines are cut off
)

// Align positions text that is not scrolling.
type Align string

const (
	AlignLeft   Align = "left"
	AlignCenter Align = "center"
	AlignRight  Align = "right"
)

// Text describes a message for a Player. Up to four lines, separated by
// "\n", fit on the panel; they are centred vertically.
type Text struct {
	Text       string
	Color      color.Color // default white
	Background color.Color // default black
	Align      Align       // default AlignCenter
	Scroll     Scroll      // default ScrollAuto
	Speed      float64     // scroll speed in pixels per second, default 24
	Images     Images      // pictures for extra :shortcodes:, e.g. Pokémon sprites
}

// WithDefaults fills in the defaults for unset fields.
func (t Text) WithDefaults() Text {
	if t.Color == nil {
		t.Color = color.White
	}
	if t.Background == nil {
		t.Background = color.Black
	}
	if t.Align == "" {
		t.Align = AlignCenter
	}
	if t.Scroll == "" {
		t.Scroll = ScrollAuto
	}
	if t.Speed <= 0 {
		t.Speed = 24
	}
	return t
}

func (t Text) lines() []string {
	lines := strings.Split(strings.ReplaceAll(t.Text, "\r\n", "\n"), "\n")
	if max := Height / LineHeight; len(lines) > max {
		lines = lines[:max]
	}
	return lines
}

// textLine is one laid-out line of a Text.
type textLine struct {
	cells         []cell
	width, height int
}

// layout splits the text into lines. Inline colour codes only colour the
// rest of their own line.
func (t Text) layout() []textLine {
	var out []textLine
	for _, l := range t.lines() {
		cs := cells(l, t.Images)
		out = append(out, textLine{cs, cellsWidth(cs), lineHeight(cs)})
	}
	return out
}

// step is how long the text takes to scroll one pixel.
func (t Text) step() time.Duration { return time.Duration(float64(time.Second) / t.Speed) }

// scrolls reports whether a line of the given width scrolls. Each line is
// decided on its own: with ScrollAuto only lines too wide for the panel move.
func (t Text) scrolls(width int) bool {
	return t.Scroll == ScrollAlways || (t.Scroll == ScrollAuto && width > Width)
}

// Render draws the text as it appears elapsed time after it was first shown
// (which only matters when it scrolls). It implements Scene.
func (t Text) Render(c *Canvas, elapsed time.Duration) time.Duration {
	t = t.WithDefaults()
	c.Fill(t.Background)
	lines := t.layout()
	total := -1
	for _, l := range lines {
		total += l.height + 1
	}
	y := (Height - total) / 2
	moving := false
	for _, l := range lines {
		var x int
		switch {
		case t.scrolls(l.width):
			// Each line enters from the right edge, leaves completely on the
			// left and repeats, looping at its own length.
			moving = true
			x = Width - int(elapsed/t.step())%(Width+l.width)
		case t.Align == AlignLeft:
			x = 0
		case t.Align == AlignRight:
			x = Width - l.width
		default:
			x = (Width - l.width) / 2
		}
		c.drawCells(x, y, l.cells, t.Color)
		y += l.height + 1
	}
	if !moving {
		return -1
	}
	// Next frame exactly when the text has moved one more pixel, so every
	// frame advances by the same amount.
	return t.step() - elapsed%t.step()
}

// Scene is anything a Player can keep on the panel.
type Scene interface {
	// Render draws the scene as it should look elapsed time after it was
	// first shown, and returns how long until it next changes, or a
	// negative duration if it is static.
	Render(c *Canvas, elapsed time.Duration) time.Duration
}

// Still is a static picture; ShowImage and Clear show one.
type Still struct{ Canvas *Canvas }

func (s Still) Render(c *Canvas, _ time.Duration) time.Duration {
	*c = *s.Canvas
	return -1
}

// State is a snapshot of what a Player is doing.
type State struct {
	Connected  bool
	Port       string
	Error      string // last connection error, if not connected
	Brightness uint8
	Scene      Scene // what is being shown
	Text       *Text // the scene, if it is text
}

// Player keeps content on the panel from a background goroutine: it animates
// scrolling text, and reconnects and redraws if the Pico is unplugged or
// reset. All methods are safe for concurrent use.
type Player struct {
	portName string

	mu         sync.Mutex
	scene      Scene
	shownAt    time.Time
	brightness uint8
	connected  bool
	port       string
	lastErr    string
	frame      Canvas // the last frame rendered, for previews
	frameSeq   uint64 // counts frames rendered

	wake chan struct{}
	done chan struct{}
}

// NewPlayer starts a Player. portName may be empty to find the Pico
// automatically.
func NewPlayer(portName string) *Player {
	p := &Player{
		portName:   portName,
		brightness: 128,
		scene:      Still{NewCanvas()},
		shownAt:    time.Now(),
		wake:       make(chan struct{}, 1),
		done:       make(chan struct{}),
	}
	go p.run()
	return p
}

// Show replaces the panel content with any Scene.
func (p *Player) Show(s Scene) { p.update(func() { p.scene = s }) }

// ShowText replaces the panel content with text.
func (p *Player) ShowText(t Text) { p.Show(t.WithDefaults()) }

// ShowImage replaces the panel content with an image, scaled down to fit.
func (p *Player) ShowImage(img image.Image) {
	c := NewCanvas()
	c.DrawImage(img)
	p.Show(Still{c})
}

// Redraw renders the current scene again straight away. Scenes whose
// content changes from outside (like a game) call it after each change.
func (p *Player) Redraw() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// Clear blanks the panel.
func (p *Player) Clear() { p.Show(Still{NewCanvas()}) }

// SetBrightness sets the panel brightness, 0-255.
func (p *Player) SetBrightness(b uint8) { p.update(func() { p.brightness = b }) }

// State reports the current content and connection status.
func (p *Player) State() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := State{Connected: p.connected, Port: p.port, Error: p.lastErr, Brightness: p.brightness, Scene: p.scene}
	if t, ok := p.scene.(Text); ok {
		s.Text = &t
	}
	return s
}

// Frame returns a copy of the last frame rendered and a number that
// changes whenever a new frame is rendered. Frames are rendered even while
// the panel is disconnected, so a preview keeps working.
func (p *Player) Frame() (Canvas, uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.frame, p.frameSeq
}

// Close stops the Player. The panel keeps showing its last frame.
func (p *Player) Close() { close(p.done) }

func (p *Player) update(f func()) {
	p.mu.Lock()
	f()
	p.shownAt = time.Now()
	p.mu.Unlock()
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *Player) setStatus(connected bool, port string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.connected, p.port, p.lastErr = connected, port, ""
	if err != nil {
		p.lastErr = err.Error()
	}
}

const (
	// Fast animations skip ahead rather than exceed this frame rate.
	minFrameInterval = time.Second / 60
	// Static content is resent this often so that an unplugged or reset
	// Pico is noticed and redrawn without waiting for the next change.
	keepaliveInterval = 2 * time.Second
)

func (p *Player) run() {
	var d *Display
	defer func() {
		if d != nil {
			d.Close()
		}
	}()
	canvas := NewCanvas()
	sentBrightness := -1
	var nextAttempt time.Time // when to try to reconnect to the panel
	for {
		if d == nil && !time.Now().Before(nextAttempt) {
			var err error
			if d, err = Open(p.portName); err != nil {
				d = nil
				p.setStatus(false, p.portName, err)
				nextAttempt = time.Now().Add(2 * time.Second)
			} else {
				p.setStatus(true, d.Port(), nil)
				sentBrightness = -1
			}
		}

		p.mu.Lock()
		brightness := p.brightness
		next := p.scene.Render(canvas, time.Since(p.shownAt))
		nextFrame := time.Now().Add(next)
		p.frame = *canvas
		p.frameSeq++
		p.mu.Unlock()

		if d != nil {
			err := d.Show(canvas)
			if err == nil && int(brightness) != sentBrightness {
				if err = d.SetBrightness(brightness); err == nil {
					sentBrightness = int(brightness)
				}
			}
			if err != nil {
				d.Close()
				d = nil
				p.setStatus(false, p.portName, err)
				continue
			}
		}

		wait := keepaliveInterval
		if next >= 0 {
			wait = min(max(time.Until(nextFrame), minFrameInterval), keepaliveInterval)
		}
		if d == nil {
			// Keep animating the preview, but come back to reconnect.
			wait = min(wait, max(time.Until(nextAttempt), minFrameInterval))
		}
		if !p.sleep(wait) {
			return
		}
	}
}

// sleep waits for d or until woken; false means the Player was closed.
func (p *Player) sleep(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-p.done:
		return false
	case <-p.wake:
	case <-t.C:
	}
	return true
}
