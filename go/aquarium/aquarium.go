// Package aquarium draws a live reef tank as a matrix.Scene: clownfish
// darting round their anemone, blue tangs cruising the length of the tank
// and a betta trailing its long fins, over sand and rocks, with swaying
// seaweed, rising bubbles and shimmering light from above.
package aquarium

import (
	"image/color"
	"math"
	"math/rand"
	"sort"
	"time"

	"dotmatrix/matrix"
)

// Scene implements matrix.Scene. The fish keep swimming between frames,
// so a Scene shouldn't be shown in two places at once.
type Scene struct {
	fish []*fish
	rng  *rand.Rand
	last time.Duration // elapsed time of the previous frame
	now  float64       // seconds simulated
}

// New returns a tank with two clownfish, two blue tangs and a betta.
func New() *Scene {
	s := &Scene{rng: rand.New(rand.NewSource(1))}
	add := func(k *kind, x, y float64) {
		f := &fish{kind: k, x: x, y: y, phase: s.rng.Float64() * 10}
		f.pickTarget(s.rng)
		s.fish = append(s.fish, f)
	}
	add(&clownfish, anemoneX-4, anemoneY-4)
	add(&clownfish, anemoneX+5, anemoneY-6)
	add(&blueTang, 10, 8)
	add(&blueTang, 50, 14)
	add(&betta, 32, 18)
	return s
}

const frameInterval = time.Second / 25

// Tank layout.
const (
	sandTop            = 28 // first row of sand
	anemoneX, anemoneY = 13, 24
)

// Render implements matrix.Scene.
func (s *Scene) Render(c *matrix.Canvas, elapsed time.Duration) time.Duration {
	dt := (elapsed - s.last).Seconds()
	if dt < 0 || dt > 0.5 { // shown afresh, or after a long pause
		dt = 0
	}
	s.last = elapsed
	s.now += dt
	t := s.now

	for _, f := range s.fish {
		f.swim(dt, s.rng)
	}
	s.separate(dt)

	drawWater(c, t)
	drawSand(c)
	drawRocks(c)
	drawSeaweed(c, t, 47, 13, 0)
	drawSeaweed(c, t, 51, 9, 1.3)
	drawSeaweed(c, t, 4, 8, 2.1)
	drawBubbles(c, t)

	// Far fish first: the betta (biggest) passes behind.
	fish := append([]*fish(nil), s.fish...)
	sort.SliceStable(fish, func(i, j int) bool { return fish[i].kind.depth < fish[j].kind.depth })
	for _, f := range fish {
		if f.kind == &clownfish {
			continue // they nestle in front of the anemone, drawn below
		}
		f.draw(c, t)
	}
	drawAnemone(c, t)
	for _, f := range fish {
		if f.kind == &clownfish {
			f.draw(c, t)
		}
	}
	drawSeaweed(c, t, 58, 11, 0.7) // one strand in front of everything
	return frameInterval
}

// separate nudges fish that are about to overlap apart, vertically, and
// sends the one behind somewhere else, so they don't swim through each
// other.
func (s *Scene) separate(dt float64) {
	for i, a := range s.fish {
		for _, b := range s.fish[i+1:] {
			aw, ah := a.size()
			bw, bh := b.size()
			dx, dy := b.x-a.x, b.y-a.y
			ox := (aw+bw)/2 + 1 - math.Abs(dx)
			oy := (ah+bh)/2 + 1 - math.Abs(dy)
			if ox <= 0 || oy <= 0 {
				continue
			}
			// Different kinds pass above or below each other; a pair of the
			// same kind may also part sideways.
			if a.kind == b.kind && ox < oy {
				push := math.Min(ox, 3) * 3 * dt
				if dx < 0 {
					push = -push
				}
				a.x -= push * a.yield(b)
				b.x += push * b.yield(a)
			} else {
				push := math.Min(oy, 3) * 3 * dt
				if dy < 0 || (dy == 0 && i%2 == 0) {
					push = -push
				}
				a.y -= push * a.yield(b)
				b.y += push * b.yield(a)
			}
			a.clamp()
			b.clamp()
			// The slower swimmer gives way.
			if a.kind.speed < b.kind.speed {
				a.giveWay(b, s.rng)
			} else {
				b.giveWay(a, s.rng)
			}
		}
	}
}

// yield is how much of a push apart f takes when it meets other: the
// slower fish moves aside, and fish of a kind share it.
func (f *fish) yield(other *fish) float64 {
	switch {
	case f.kind.speed < other.kind.speed:
		return 1
	case f.kind.speed > other.kind.speed:
		return 0
	}
	return 0.5
}

func (f *fish) size() (w, h float64) {
	return float64(len(f.kind.frames[0][0])), float64(len(f.kind.frames[0]))
}

func (f *fish) clamp() {
	x0, y0, x1, y1 := f.bounds()
	f.x = math.Max(x0, math.Min(x1, f.x))
	f.y = math.Max(y0, math.Min(y1, f.y))
}

// giveWay sends f to a new spot away from other, if it is heading past it.
func (f *fish) giveWay(other *fish, r *rand.Rand) {
	if f.cooldown > 0 {
		return
	}
	f.cooldown = 1.5
	if math.Abs(f.tx-other.x) > 12 && (f.tx-f.x)*(other.x-f.x) <= 0 {
		return // already heading away
	}
	x0, y0, x1, y1 := f.bounds()
	for range 4 {
		f.pickTarget(r)
		if math.Abs(f.tx-other.x) > 14 {
			break
		}
	}
	f.tx = math.Max(x0, math.Min(x1, f.tx))
	f.ty = math.Max(y0, math.Min(y1, f.ty))
}

// --- Fish ---

// kind is a species: its sprite frames (facing right) and how it swims.
type kind struct {
	frames  [][]string // tail/fin animation frames
	palette map[byte]color.RGBA
	speed   float64     // pixels per second, cruising
	wag     float64     // animation frames per second at cruising speed
	depth   int         // drawing order: lower is further back
	home    *[4]float64 // x0, y0, x1, y1 the fish keeps to, or nil for the whole tank
}

type fish struct {
	kind     *kind
	x, y     float64 // centre
	vx, vy   float64
	tx, ty   float64 // where it's heading
	left     bool    // facing left
	pause    float64 // seconds left to hover before moving on
	cooldown float64 // seconds before it will change course for another fish again
	patience float64 // seconds left to reach the target before choosing another
	phase    float64
}

func (f *fish) bounds() (x0, y0, x1, y1 float64) {
	w, h := float64(len(f.kind.frames[0][0])), float64(len(f.kind.frames[0]))
	if r := f.kind.home; r != nil {
		return r[0], r[1], r[2], r[3]
	}
	return w/2 + 1, h/2 + 1, matrix.Width - w/2 - 1, sandTop - h/2
}

func (f *fish) pickTarget(r *rand.Rand) {
	x0, y0, x1, y1 := f.bounds()
	f.patience = 4 + 2*math.Hypot(x1-x0, y1-y0)/f.kind.speed
	for range 6 {
		f.tx = x0 + r.Float64()*(x1-x0)
		// Cruisers head well along the tank rather than nudging about.
		if f.kind.home != nil || math.Abs(f.tx-f.x) > 20 {
			break
		}
	}
	f.ty = y0 + r.Float64()*(y1-y0)
	// Cruisers mostly swim the length of the tank rather than bobbing up
	// and down.
	if f.kind.home == nil {
		f.ty = f.y + (f.ty-f.y)*0.4
	}
}

func (f *fish) swim(dt float64, r *rand.Rand) {
	if dt == 0 {
		return
	}
	f.cooldown -= dt
	// Give up on a target it can't reach, e.g. with another fish in the way.
	if f.patience -= dt; f.patience < 0 {
		f.pickTarget(r)
	}
	dx, dy := f.tx-f.x, f.ty-f.y
	dist := math.Hypot(dx, dy)
	if dist < 1.5 {
		if f.pause <= 0 {
			f.pause = 0.3 + r.Float64()*2
		}
		f.pause -= dt
		if f.pause <= 0 {
			f.pickTarget(r)
		}
		dx, dy = 0, 0
	}
	// Steer smoothly towards the target, slowing as it arrives.
	want := f.kind.speed * math.Min(1, dist/6)
	var wx, wy float64
	if dist > 0 && (dx != 0 || dy != 0) {
		wx, wy = dx/dist*want, dy/dist*want*0.6
	}
	k := 1 - math.Exp(-dt*1.8)
	f.vx += (wx - f.vx) * k
	f.vy += (wy - f.vy) * k
	f.x += f.vx * dt
	f.y += f.vy * dt
	// Turn round only when clearly swimming the other way.
	if f.vx < -0.6 {
		f.left = true
	} else if f.vx > 0.6 {
		f.left = false
	}
	f.phase += dt * f.kind.wag * (0.4 + math.Hypot(f.vx, f.vy)/f.kind.speed)
}

func (f *fish) draw(c *matrix.Canvas, t float64) {
	frame := f.kind.frames[int(f.phase)%len(f.kind.frames)]
	h, w := len(frame), len(frame[0])
	bob := 0.4 * math.Sin(t*1.7+f.phase*0.1)
	x0 := int(math.Round(f.x - float64(w)/2))
	y0 := int(math.Round(f.y + bob - float64(h)/2))
	for row, line := range frame {
		for col := 0; col < w; col++ {
			ch := line[col]
			if f.left {
				ch = line[w-1-col]
			}
			if p, ok := f.kind.palette[ch]; ok {
				c.Set(x0+col, y0+row, p)
			}
		}
	}
}

var (
	orange  = color.RGBA{255, 110, 0, 255}
	white   = color.RGBA{245, 245, 245, 255}
	black   = color.RGBA{0, 0, 0, 255}
	eye     = color.RGBA{10, 10, 20, 255}
	royal   = color.RGBA{20, 70, 255, 255}
	sky     = color.RGBA{80, 150, 255, 255}
	yellow  = color.RGBA{255, 220, 0, 255}
	navy    = color.RGBA{0, 0, 40, 255}
	crimson = color.RGBA{170, 0, 35, 255}
	scarlet = color.RGBA{255, 40, 60, 255}
	violet  = color.RGBA{170, 40, 255, 255}
	wine    = color.RGBA{200, 0, 110, 255}
)

var clownfish = kind{
	frames: [][]string{{
		"....kook....",
		"k.owoowwowo.",
		"ooowoowwoweo",
		"ooowoowwowoo",
		"k.owoowwowo.",
		"....kook....",
	}, {
		"....kook....",
		"..owoowwowo.",
		"koowoowwoweo",
		"koowoowwowoo",
		"..owoowwowo.",
		"....kook....",
	}},
	palette: map[byte]color.RGBA{'o': orange, 'w': white, 'k': black, 'e': eye},
	speed:   7, wag: 8, depth: 2,
	home: &[4]float64{anemoneX - 7, anemoneY - 13, anemoneX + 13, anemoneY - 3},
}

var blueTang = kind{
	frames: [][]string{{
		"....bbbbb...",
		"y..bnnnnbb..",
		"yybbbbbnnbb.",
		"yyybbbbbbbeb",
		"yybnnnbbbbb.",
		"y..bbnnbbb..",
		"....bbbb....",
	}, {
		"....bbbbb...",
		"...bnnnnbb..",
		"yybbbbbnnbb.",
		"yyybbbbbbbeb",
		"yybnnnbbbbb.",
		"...bbnnbbb..",
		"....bbbb....",
	}},
	palette: map[byte]color.RGBA{'b': royal, 'l': sky, 'n': navy, 'y': yellow, 'e': eye},
	speed:   9, wag: 5, depth: 1,
}

var betta = kind{
	frames: [][]string{{
		"..vvv.........",
		".vsssv..ss....",
		"vswsswvsssss..",
		"vssswsrrrrrrer",
		"vswsssrrrrrrrr",
		"vsswsrrrrrrrr.",
		".vsswssssss...",
		"..vssvsssv....",
		"...vv..vv.....",
	}, {
		".vvv..........",
		"vsssv...ss....",
		"vswssvsssss...",
		"vsswssrrrrrrer",
		"vswsssrrrrrrrr",
		"vssswsrrrrrrr.",
		"vsswsssssss...",
		".vsssvssv.....",
		"..vvv..vv.....",
	}},
	palette: map[byte]color.RGBA{'r': crimson, 's': scarlet, 'v': violet, 'w': wine, 'e': eye},
	speed:   4, wag: 2.5, depth: 0,
}

// --- Scenery ---

func drawWater(c *matrix.Canvas, t float64) {
	for y := 0; y < sandTop; y++ {
		d := float64(y) / sandTop
		for x := 0; x < matrix.Width; x++ {
			fx := float64(x)
			// Sunlight shafts slanting down from the surface, swaying.
			ray := math.Pow(math.Max(0, math.Sin(fx*0.22+float64(y)*0.12+math.Sin(t*0.4)*0.8)), 6) * (1 - d) * 0.5
			// Caustic shimmer near the surface.
			sh := 0.0
			if y < 3 {
				sh = math.Max(0, math.Sin(fx*0.7+t*2.3)+math.Sin(fx*0.31-t*1.7)-0.8) * (0.25 - 0.08*float64(y))
			}
			l := ray + sh
			c.Pix[y][x] = color.RGBA{
				uint8(clamp(0 + 120*l)),
				uint8(clamp(60 - 35*d + 160*l)),
				uint8(clamp(130 - 70*d + 160*l)),
				255,
			}
		}
	}
}

func clamp(v float64) float64 { return math.Max(0, math.Min(255, v)) }

// Sand: a gentle dune, speckled.
func drawSand(c *matrix.Canvas) {
	r := rand.New(rand.NewSource(3))
	for x := 0; x < matrix.Width; x++ {
		top := sandTop + int(math.Round(0.8*math.Sin(float64(x)*0.15+1)))
		for y := top; y < matrix.Height; y++ {
			v := r.Intn(30)
			c.Pix[y][x] = color.RGBA{uint8(170 + v), uint8(140 + v), uint8(90 + v/2), 255}
		}
	}
}

// rocks are drawn as filled ellipses: centre x, centre y, radii, shade.
var rocks = [][5]float64{
	{13, 27, 7, 3.5, 85}, {22, 28.5, 4, 2.5, 70}, {40, 28, 5, 3, 95}, {60, 28.5, 4, 2.2, 75},
}

func drawRocks(c *matrix.Canvas) {
	for _, r := range rocks {
		for y := int(r[1] - r[3]); y <= int(r[1]+r[3]); y++ {
			for x := int(r[0] - r[2]); x <= int(r[0]+r[2]); x++ {
				dx, dy := (float64(x)-r[0])/r[2], (float64(y)-r[1])/r[3]
				if dx*dx+dy*dy > 1 {
					continue
				}
				// Lit from above.
				v := r[4] * (1.25 - 0.45*(dy+1)/2)
				c.Set(x, y, color.RGBA{uint8(v), uint8(v * 0.92), uint8(v * 0.85), 255})
			}
		}
	}
}

// drawAnemone draws a pink-tipped anemone whose tentacles sway.
func drawAnemone(c *matrix.Canvas, t float64) {
	base := color.RGBA{170, 40, 120, 255}
	tip := color.RGBA{255, 120, 220, 255}
	for i := -5; i <= 5; i++ {
		lean := float64(i) * 0.35
		h := 5 - abs(i)/2
		for k := 0; k < h; k++ {
			sway := math.Sin(t*1.6+float64(i)*0.5-float64(k)*0.5) * float64(k) * 0.25
			x := anemoneX + i + int(math.Round(lean*float64(k)/float64(h)+sway))
			col := base
			if k == h-1 {
				col = tip
			}
			c.Set(x, anemoneY-k, col)
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// drawSeaweed draws a strand of kelp rising from the sand at x, waving.
func drawSeaweed(c *matrix.Canvas, t, x float64, height int, phase float64) {
	dark, light := color.RGBA{0, 110, 40, 255}, color.RGBA{40, 190, 70, 255}
	for k := 0; k < height; k++ {
		sway := math.Sin(t*1.1+phase-float64(k)*0.35) * float64(k) * 0.18
		px := int(math.Round(x + sway))
		y := sandTop + 1 - k
		col := dark
		if (k/2)%2 == 0 {
			col = light
		}
		c.Set(px, y, col)
		// A leaf every few pixels, alternating sides.
		if k%3 == 1 {
			side := 1
			if (k/3)%2 == 0 {
				side = -1
			}
			c.Set(px+side, y, dark)
		}
	}
}

// Bubbles rise from a spot in the sand, wobbling.
func drawBubbles(c *matrix.Canvas, t float64) {
	col := color.RGBA{190, 230, 255, 255}
	for i := 0; i < 6; i++ {
		fi := float64(i)
		life := math.Mod(t*0.9+fi*0.83, 5.0) / 5.0 // 0..1 from sand to surface
		y := float64(sandTop-1) - life*float64(sandTop)
		x := 36 + math.Sin(t*3+fi*1.7)*0.9 + fi*0.15
		c.Set(int(math.Round(x)), int(math.Round(y)), col)
		if i%3 == 0 && life > 0.4 { // bigger bubbles near the top
			c.Set(int(math.Round(x))+1, int(math.Round(y)), col)
		}
	}
}
