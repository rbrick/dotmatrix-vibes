// Package tetris implements a controller-operated falling-block game.
package tetris

import (
	"image/color"
	"math/rand"
	"sync"
	"time"

	"dotmatrix/matrix"
)

type point struct{ x, y int }
type piece struct{ kind, rot, x, y int }

var shapes = [7][4][4]point{
	{{{0, 1}, {1, 1}, {2, 1}, {3, 1}}, {{2, 0}, {2, 1}, {2, 2}, {2, 3}}, {{0, 2}, {1, 2}, {2, 2}, {3, 2}}, {{1, 0}, {1, 1}, {1, 2}, {1, 3}}},
	{{{1, 0}, {2, 0}, {1, 1}, {2, 1}}, {{1, 0}, {2, 0}, {1, 1}, {2, 1}}, {{1, 0}, {2, 0}, {1, 1}, {2, 1}}, {{1, 0}, {2, 0}, {1, 1}, {2, 1}}},
	{{{1, 0}, {0, 1}, {1, 1}, {2, 1}}, {{1, 0}, {1, 1}, {2, 1}, {1, 2}}, {{0, 1}, {1, 1}, {2, 1}, {1, 2}}, {{1, 0}, {0, 1}, {1, 1}, {1, 2}}},
	{{{1, 0}, {2, 0}, {0, 1}, {1, 1}}, {{1, 0}, {1, 1}, {2, 1}, {2, 2}}, {{1, 1}, {2, 1}, {0, 2}, {1, 2}}, {{0, 0}, {0, 1}, {1, 1}, {1, 2}}},
	{{{0, 0}, {1, 0}, {1, 1}, {2, 1}}, {{2, 0}, {1, 1}, {2, 1}, {1, 2}}, {{0, 1}, {1, 1}, {1, 2}, {2, 2}}, {{1, 0}, {0, 1}, {1, 1}, {0, 2}}},
	{{{0, 0}, {0, 1}, {1, 1}, {2, 1}}, {{1, 0}, {2, 0}, {1, 1}, {1, 2}}, {{0, 1}, {1, 1}, {2, 1}, {2, 2}}, {{1, 0}, {1, 1}, {0, 2}, {1, 2}}},
	{{{2, 0}, {0, 1}, {1, 1}, {2, 1}}, {{1, 0}, {1, 1}, {1, 2}, {2, 2}}, {{0, 1}, {1, 1}, {2, 1}, {0, 2}}, {{0, 0}, {1, 0}, {1, 1}, {1, 2}}},
}
var colors = [8]color.RGBA{{}, {0, 220, 255, 255}, {255, 220, 0, 255}, {180, 0, 255, 255}, {0, 230, 90, 255}, {255, 40, 50, 255}, {30, 90, 255, 255}, {255, 130, 0, 255}}

type Game struct {
	mu           sync.Mutex
	board        [16][10]uint8
	cur          piece
	next         int
	score, lines int
	over         bool
	last         time.Time
	rng          *rand.Rand
}

func New() *Game {
	g := &Game{rng: rand.New(rand.NewSource(time.Now().UnixNano())), last: time.Now()}
	g.next = g.rng.Intn(7)
	g.spawn()
	return g
}
func (g *Game) spawn() {
	g.cur = piece{kind: g.next, x: 3, y: -1}
	g.next = g.rng.Intn(7)
	if g.collides(g.cur) {
		g.over = true
	}
}
func (g *Game) collides(p piece) bool {
	for _, q := range shapes[p.kind][p.rot] {
		x, y := p.x+q.x, p.y+q.y
		if x < 0 || x >= 10 || y >= 16 || (y >= 0 && g.board[y][x] != 0) {
			return true
		}
	}
	return false
}
func (g *Game) move(dx, dy int) bool {
	p := g.cur
	p.x += dx
	p.y += dy
	if g.collides(p) {
		return false
	}
	g.cur = p
	return true
}
func (g *Game) Move(dx int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.over {
		g.move(dx, 0)
	}
}
func (g *Game) Rotate() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.over {
		return
	}
	p := g.cur
	p.rot = (p.rot + 1) % 4
	for _, kick := range []int{0, -1, 1, -2, 2} {
		p.x = g.cur.x + kick
		if !g.collides(p) {
			g.cur = p
			return
		}
	}
}
func (g *Game) SoftDrop() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.over && !g.move(0, 1) {
		g.lock()
	}
}
func (g *Game) HardDrop() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.over {
		return
	}
	for g.move(0, 1) {
		g.score += 2
	}
	g.lock()
}
func (g *Game) lock() {
	for _, q := range shapes[g.cur.kind][g.cur.rot] {
		x, y := g.cur.x+q.x, g.cur.y+q.y
		if y >= 0 {
			g.board[y][x] = uint8(g.cur.kind + 1)
		}
	}
	cleared := 0
	for y := 15; y >= 0; y-- {
		full := true
		for x := 0; x < 10; x++ {
			if g.board[y][x] == 0 {
				full = false
				break
			}
		}
		if full {
			for yy := y; yy > 0; yy-- {
				g.board[yy] = g.board[yy-1]
			}
			g.board[0] = [10]uint8{}
			cleared++
			y++
		}
	}
	g.lines += cleared
	g.score += []int{0, 100, 300, 500, 800}[cleared]
	g.spawn()
}
func (g *Game) Restart() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.board = [16][10]uint8{}
	g.score = 0
	g.lines = 0
	g.over = false
	g.spawn()
	g.last = time.Now()
}
func (g *Game) Render(c *matrix.Canvas, _ time.Duration) time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	step := 700*time.Millisecond - time.Duration(min(g.lines, 12))*40*time.Millisecond
	if !g.over && now.Sub(g.last) >= step {
		if !g.move(0, 1) {
			g.lock()
		}
		g.last = now
	}
	c.Clear()
	ox := 15
	for y := 0; y < 16; y++ {
		for x := 0; x < 10; x++ {
			if v := g.board[y][x]; v > 0 {
				block(c, ox+x*2, y*2, colors[v])
			}
		}
	}
	if !g.over {
		for _, q := range shapes[g.cur.kind][g.cur.rot] {
			if y := g.cur.y + q.y; y >= 0 {
				block(c, ox+(g.cur.x+q.x)*2, y*2, colors[g.cur.kind+1])
			}
		}
	}
	white := color.RGBA{255, 255, 255, 255}
	c.DrawText(0, 1, "BLOCK", white)
	c.DrawText(0, 9, "DROP", white)
	c.DrawText(38, 1, "SCORE", white)
	c.DrawText(40, 9, itoa(g.score), colors[2])
	c.DrawText(40, 18, "L"+itoa(g.lines), white)
	if g.over {
		c.DrawText(20, 12, "GAME", white)
		c.DrawText(20, 20, "OVER", colors[5])
	}
	return 33 * time.Millisecond
}
func block(c *matrix.Canvas, x, y int, col color.Color) {
	c.Set(x, y, col)
	c.Set(x+1, y, col)
	c.Set(x, y+1, col)
	c.Set(x+1, y+1, col)
}
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := [12]byte{}
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
