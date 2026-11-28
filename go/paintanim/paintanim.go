// Package paintanim animates a picture being painted, the way a painter
// works: a toned ground washed across the canvas, a loose block-in of the
// big colour masses with a large brush, then smaller and smaller brushes
// refining what is still wrong, and finally single-pixel touches that land
// exactly on the picture.
//
// Strokes follow Hertzmann's painterly rendering ("Painterly Rendering with
// Curved Brush Strokes of Multiple Sizes", SIGGRAPH 1998): each layer paints
// a blurrier copy of the picture with a bigger brush, placing strokes only
// where the canvas is still noticeably off, and each stroke runs along the
// forms (across the brightness gradient) for as long as its colour still
// suits the picture. Within a layer, darks go down before lights, as in oil
// painting, and every stroke is drawn progressively along its path.
package paintanim

import (
	"hash/fnv"
	"image/color"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"

	"dotmatrix/matrix"
)

// Options control the painting.
type Options struct {
	Seconds float64 // how long painting takes; default 30
	Loop    bool    // paint again (with new strokes) after holding the finished picture
	Hold    float64 // seconds the finished picture stays before a loop repaints; default 20
}

func (o Options) withDefaults() Options {
	if o.Seconds <= 0 {
		o.Seconds = 30
	}
	if o.Hold <= 0 {
		o.Hold = 20
	}
	return o
}

const (
	W, H      = matrix.Width, matrix.Height
	frameTime = 50 * time.Millisecond
)

// rgb is a colour with float channels, 0-255.
type rgb [3]float64

func (c rgb) dist(d rgb) float64 {
	return math.Sqrt((c[0]-d[0])*(c[0]-d[0]) + (c[1]-d[1])*(c[1]-d[1]) + (c[2]-d[2])*(c[2]-d[2]))
}

func (c rgb) lum() float64 { return 0.299*c[0] + 0.587*c[1] + 0.114*c[2] }

type img [H][W]rgb

// dab is one pixel a stroke touches: where, how strongly, and how far along
// the stroke (0-1) it is, so the stroke can be drawn progressively.
type dab struct {
	x, y  int
	alpha float64
	along float64
}

type stroke struct {
	col        rgb
	dabs       []dab   // sorted by along
	start, end float64 // seconds into the painting
}

// plan is one complete painting: its strokes, in order.
type plan struct{ strokes []stroke }

// Scene shows the picture being painted. It implements matrix.Scene.
type Scene struct {
	target  *matrix.Canvas
	targetF img
	opts    Options
	seed    int64

	mu        sync.Mutex
	round     int     // which painting (loops paint again with new strokes)
	plan      *plan   // the strokes of this round
	canvas    img     // the canvas with plan.strokes[:done] applied
	done      int     // strokes fully applied to canvas
	planned   bool    // plan is for round
	lastFrame float64 // seconds into the round of the last render
}

// New returns a Scene that paints target.
func New(target *matrix.Canvas, o Options) *Scene {
	s := &Scene{target: target, opts: o.withDefaults()}
	h := fnv.New64a()
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			p := target.Pix[y][x]
			s.targetF[y][x] = rgb{float64(p.R), float64(p.G), float64(p.B)}
			h.Write([]byte{p.R, p.G, p.B})
		}
	}
	s.seed = int64(h.Sum64())
	return s
}

// Target is the finished picture.
func (s *Scene) Target() *matrix.Canvas { return s.target }

// Options returns the scene's options, with defaults filled in.
func (s *Scene) Options() Options { return s.opts }

// Render implements matrix.Scene.
func (s *Scene) Render(c *matrix.Canvas, elapsed time.Duration) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := elapsed.Seconds()
	round := 0
	if s.opts.Loop {
		period := s.opts.Seconds + s.opts.Hold
		round = int(t / period)
		t -= float64(round) * period
	}
	if t >= s.opts.Seconds {
		*c = *s.target
		if !s.opts.Loop {
			return -1
		}
		return time.Duration((s.opts.Seconds + s.opts.Hold - t) * float64(time.Second))
	}
	if !s.planned || round != s.round || t < s.lastFrame {
		s.round, s.planned = round, true
		s.plan = s.makePlan(s.seed + int64(round))
		s.canvas = img{} // bare (black) canvas; the plan starts with the ground washes
		s.done = 0
	}
	s.lastFrame = t

	// Apply finished strokes, then draw the one in progress partly.
	for s.done < len(s.plan.strokes) && s.plan.strokes[s.done].end <= t {
		apply(&s.canvas, &s.plan.strokes[s.done], 1)
		s.done++
	}
	frame := s.canvas
	if s.done < len(s.plan.strokes) {
		if st := &s.plan.strokes[s.done]; st.start <= t {
			apply(&frame, st, (t-st.start)/(st.end-st.start))
		}
	}
	toCanvas(c, &frame)
	return frameTime
}

// The painting's timeline, as fractions of Seconds.
const (
	groundEnd = 0.06 // the toned ground is washed on
)

// layers are the brush sizes, from block-in to fine work, with the share of
// the painting time each gets. The last is single-pixel detail work.
var layers = []struct {
	radius, blur, share, threshold float64
	maxLen                         int
}{
	{radius: 4, blur: 3, share: 0.30, threshold: 28, maxLen: 9},
	{radius: 2.2, blur: 1.5, share: 0.26, threshold: 22, maxLen: 7},
	{radius: 1.2, blur: 0.7, share: 0.22, threshold: 16, maxLen: 5},
	{radius: 0, blur: 0, share: 0.16, threshold: 6, maxLen: 1},
}

// groundColour is the toned ground: the picture's average, a little
// muted, like an imprimatura.
func (s *Scene) groundColour() rgb {
	var sum rgb
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			for i := range sum {
				sum[i] += s.targetF[y][x][i]
			}
		}
	}
	var g rgb
	for i := range g {
		g[i] = sum[i] / (W * H)
	}
	l := g.lum()
	for i := range g {
		g[i] = l + (g[i]-l)*0.6
	}
	return g
}

// ground is the canvas once the toned ground is on.
func (s *Scene) ground() img {
	var c img
	g := s.groundColour()
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			c[y][x] = g
		}
	}
	return c
}

// makePlan works out every stroke of one painting by painting it on a
// scratch canvas.
func (s *Scene) makePlan(seed int64) *plan {
	rng := rand.New(rand.NewSource(seed))
	total := s.opts.Seconds
	p := &plan{}

	// The ground: broad horizontal washes from the top down. The canvas
	// starts black; each wash covers a band.
	var canvas img
	g := s.groundColour()
	const bands = 4
	for b := 0; b < bands; b++ {
		y0 := b * H / bands
		y1 := (b + 1) * H / bands
		st := stroke{col: g}
		for y := y0; y < y1; y++ {
			for x := 0; x < W; x++ {
				st.dabs = append(st.dabs, dab{x, y, 1, float64(x) / W})
			}
		}
		sortDabs(&st)
		st.start = total * groundEnd * float64(b) / bands
		st.end = total * groundEnd * float64(b+1) / bands
		p.strokes = append(p.strokes, st)
	}
	canvas = s.ground()

	t := total * groundEnd
	for _, l := range layers {
		ref := blur(&s.targetF, l.blur)
		var strokes []stroke
		if l.radius == 0 {
			strokes = detailStrokes(&canvas, &ref, l.threshold)
		} else {
			strokes = brushStrokes(&canvas, &ref, l.radius, l.threshold, l.maxLen, rng)
		}
		// Darks before lights, loosely: sort by brightness with jitter.
		keys := make([]float64, len(strokes))
		for i := range strokes {
			keys[i] = strokes[i].col.lum() + rng.Float64()*90
		}
		idx := make([]int, len(strokes))
		for i := range idx {
			idx[i] = i
		}
		sort.Slice(idx, func(a, b int) bool { return keys[idx[a]] < keys[idx[b]] })

		// Each stroke takes time in proportion to its length.
		var weight float64
		for _, i := range idx {
			weight += strokeWeight(&strokes[i])
		}
		span := total * l.share
		for _, i := range idx {
			st := strokes[i]
			d := 0.0
			if weight > 0 {
				d = span * strokeWeight(&st) / weight
			}
			st.start, st.end = t, t+d
			t += d
			apply(&canvas, &st, 1)
			p.strokes = append(p.strokes, st)
		}
		if len(strokes) == 0 {
			t += span
		}
	}
	return p
}

func strokeWeight(st *stroke) float64 {
	if len(st.dabs) == 0 {
		return 0
	}
	return 1 + st.dabs[len(st.dabs)-1].along*float64(len(st.dabs))*0.15
}

// brushStrokes places curved strokes of the given radius wherever the
// canvas differs from ref by more than threshold over a grid cell, following
// Hertzmann. They are worked out against the canvas as it is before the
// layer starts.
func brushStrokes(canvas, ref *img, radius, threshold float64, maxLen int, rng *rand.Rand) []stroke {
	grid := max(1, int(math.Round(radius)))
	lum := luminance(ref)
	var out []stroke
	for gy := 0; gy < H; gy += grid {
		for gx := 0; gx < W; gx += grid {
			// The cell's average error, and its worst pixel.
			var sum, worst float64
			wx, wy, n := gx, gy, 0
			for y := gy; y < min(gy+grid, H); y++ {
				for x := gx; x < min(gx+grid, W); x++ {
					e := canvas[y][x].dist(ref[y][x])
					sum += e
					n++
					if e > worst {
						worst, wx, wy = e, x, y
					}
				}
			}
			if sum/float64(n) < threshold {
				continue
			}
			out = append(out, curvedStroke(wx, wy, radius, maxLen, canvas, ref, &lum, rng))
		}
	}
	return out
}

// curvedStroke starts at (x0, y0) in ref's colour there and runs
// perpendicular to the brightness gradient for as long as its colour suits
// the picture better than the canvas does.
func curvedStroke(x0, y0 int, radius float64, maxLen int, canvas, ref *img, lum *[H][W]float64, rng *rand.Rand) stroke {
	col := ref[y0][x0]
	pts := [][2]float64{{float64(x0), float64(y0)}}
	x, y := float64(x0), float64(y0)
	var dx, dy float64
	step := math.Max(1, radius)
	for i := 1; i < maxLen; i++ {
		ix, iy := clampi(int(math.Round(x)), 0, W-1), clampi(int(math.Round(y)), 0, H-1)
		if i > 1 && ref[iy][ix].dist(col) > ref[iy][ix].dist(canvas[iy][ix]) {
			break
		}
		gx, gy := sobel(lum, ix, iy)
		if math.Hypot(gx, gy) < 4 {
			if dx == 0 && dy == 0 {
				// Flat area and no direction yet: a loose diagonal-ish sweep.
				a := rng.Float64()*0.8 - 0.4
				dx, dy = math.Cos(a), math.Sin(a)
			}
		} else {
			nx, ny := -gy, gx // along the edge
			if nx*dx+ny*dy < 0 {
				nx, ny = -nx, -ny
			}
			l := math.Hypot(nx, ny)
			nx, ny = nx/l, ny/l
			if dx != 0 || dy != 0 {
				nx, ny = 0.6*nx+0.4*dx, 0.6*ny+0.4*dy // smooth the curve
				l = math.Hypot(nx, ny)
				nx, ny = nx/l, ny/l
			}
			dx, dy = nx, ny
		}
		x, y = x+dx*step, y+dy*step
		if x < -radius || x > W-1+radius || y < -radius || y > H-1+radius {
			break
		}
		pts = append(pts, [2]float64{x, y})
	}
	return rasterize(pts, radius, col, 0.85)
}

// rasterize turns a brush path into dabs: every pixel within radius of the
// path, with soft edges.
func rasterize(pts [][2]float64, radius float64, col rgb, opacity float64) stroke {
	// Cumulative length along the path, to order the dabs.
	cum := make([]float64, len(pts))
	for i := 1; i < len(pts); i++ {
		cum[i] = cum[i-1] + math.Hypot(pts[i][0]-pts[i-1][0], pts[i][1]-pts[i-1][1])
	}
	length := math.Max(cum[len(cum)-1], 1e-9)
	minX, maxX := int(math.Floor(pts[0][0]-radius-1)), int(math.Ceil(pts[0][0]+radius+1))
	minY, maxY := int(math.Floor(pts[0][1]-radius-1)), int(math.Ceil(pts[0][1]+radius+1))
	for _, p := range pts {
		minX, maxX = min(minX, int(math.Floor(p[0]-radius-1))), max(maxX, int(math.Ceil(p[0]+radius+1)))
		minY, maxY = min(minY, int(math.Floor(p[1]-radius-1))), max(maxY, int(math.Ceil(p[1]+radius+1)))
	}
	st := stroke{col: col}
	for y := max(minY, 0); y <= min(maxY, H-1); y++ {
		for x := max(minX, 0); x <= min(maxX, W-1); x++ {
			d, along := distToPath(float64(x), float64(y), pts, cum)
			a := opacity * clampf(radius+0.5-d, 0, 1)
			if a <= 0 {
				continue
			}
			st.dabs = append(st.dabs, dab{x, y, a, along / length})
		}
	}
	sortDabs(&st)
	return st
}

// detailStrokes are single-pixel touches wherever the canvas is still off,
// in the picture's exact colour.
func detailStrokes(canvas, ref *img, threshold float64) []stroke {
	var out []stroke
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			if canvas[y][x].dist(ref[y][x]) > threshold {
				out = append(out, stroke{col: ref[y][x], dabs: []dab{{x, y, 1, 0}}})
			}
		}
	}
	return out
}

// apply paints the first fraction (by along) of a stroke onto c.
func apply(c *img, st *stroke, fraction float64) {
	if fraction <= 0 {
		return
	}
	for _, d := range st.dabs {
		if d.along > fraction && fraction < 1 {
			break
		}
		p := &c[d.y][d.x]
		for i := range p {
			p[i] += (st.col[i] - p[i]) * d.alpha
		}
	}
}

func sortDabs(st *stroke) {
	sort.SliceStable(st.dabs, func(a, b int) bool { return st.dabs[a].along < st.dabs[b].along })
}

func distToPath(x, y float64, pts [][2]float64, cum []float64) (dist, along float64) {
	if len(pts) == 1 {
		return math.Hypot(x-pts[0][0], y-pts[0][1]), 0
	}
	dist = math.Inf(1)
	for i := 1; i < len(pts); i++ {
		ax, ay := pts[i-1][0], pts[i-1][1]
		bx, by := pts[i][0], pts[i][1]
		vx, vy := bx-ax, by-ay
		l2 := vx*vx + vy*vy
		t := 0.0
		if l2 > 0 {
			t = clampf(((x-ax)*vx+(y-ay)*vy)/l2, 0, 1)
		}
		d := math.Hypot(x-(ax+t*vx), y-(ay+t*vy))
		if d < dist {
			dist, along = d, cum[i-1]+t*math.Sqrt(l2)
		}
	}
	return dist, along
}

func luminance(m *img) [H][W]float64 {
	var l [H][W]float64
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			l[y][x] = m[y][x].lum()
		}
	}
	return l
}

func sobel(l *[H][W]float64, x, y int) (gx, gy float64) {
	at := func(x, y int) float64 { return l[clampi(y, 0, H-1)][clampi(x, 0, W-1)] }
	gx = (at(x+1, y-1) + 2*at(x+1, y) + at(x+1, y+1)) - (at(x-1, y-1) + 2*at(x-1, y) + at(x-1, y+1))
	gy = (at(x-1, y+1) + 2*at(x, y+1) + at(x+1, y+1)) - (at(x-1, y-1) + 2*at(x, y-1) + at(x+1, y-1))
	return gx / 4, gy / 4
}

// blur is a separable Gaussian blur with the given sigma, clamping at the
// edges. Sigma 0 returns the picture unchanged.
func blur(m *img, sigma float64) img {
	if sigma <= 0 {
		return *m
	}
	r := int(math.Ceil(sigma * 2.5))
	k := make([]float64, 2*r+1)
	var sum float64
	for i := range k {
		d := float64(i - r)
		k[i] = math.Exp(-d * d / (2 * sigma * sigma))
		sum += k[i]
	}
	for i := range k {
		k[i] /= sum
	}
	var tmp, out img
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			var c rgb
			for i, w := range k {
				p := m[y][clampi(x+i-r, 0, W-1)]
				for j := range c {
					c[j] += p[j] * w
				}
			}
			tmp[y][x] = c
		}
	}
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			var c rgb
			for i, w := range k {
				p := tmp[clampi(y+i-r, 0, H-1)][x]
				for j := range c {
					c[j] += p[j] * w
				}
			}
			out[y][x] = c
		}
	}
	return out
}

func toCanvas(c *matrix.Canvas, m *img) {
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			p := m[y][x]
			c.Pix[y][x] = color.RGBA{u8(p[0]), u8(p[1]), u8(p[2]), 255}
		}
	}
}

func u8(v float64) uint8 { return uint8(clampf(math.Round(v), 0, 255)) }

func clampf(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func clampi(v, lo, hi int) int { return max(lo, min(hi, v)) }
