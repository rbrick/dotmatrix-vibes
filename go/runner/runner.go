// Package runner implements an original side-scrolling pixel platform game.
package runner

import (
	"image/color"
	"sync"
	"time"

	"dotmatrix/matrix"
)

type platform struct{ x, y, w int }
type Game struct {
	mu        sync.Mutex
	x, y, vy  float64
	camera    int
	coins     map[int]bool
	won, over bool
	last      time.Time
}

var floors = []platform{{0, 28, 90}, {98, 28, 42}, {148, 28, 55}, {212, 28, 80}, {18, 21, 18}, {48, 17, 14}, {112, 20, 18}, {158, 16, 16}, {190, 21, 16}, {232, 17, 20}}
var coinX = []int{24, 54, 118, 164, 196, 240, 270}
var spikes = []int{70, 126, 180, 218}

func New() *Game { g := &Game{}; g.Restart(); return g }
func (g *Game) Restart() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.x = 3
	g.y = 24
	g.vy = 0
	g.camera = 0
	g.coins = map[int]bool{}
	g.won = false
	g.over = false
	g.last = time.Now()
}
func (g *Game) Move(dx int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.over || g.won {
		return
	}
	nx := g.x + float64(dx*3)
	if nx >= 0 && nx < 285 {
		g.x = nx
	}
	g.collect()
}
func (g *Game) Jump() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.over && !g.won && g.grounded() {
		g.vy = -18
	}
}
func (g *Game) grounded() bool {
	feet := int(g.y) + 4
	px := int(g.x)
	for _, p := range floors {
		if px+2 >= p.x && px < p.x+p.w && feet >= p.y-1 && feet <= p.y+1 {
			return true
		}
	}
	return false
}
func (g *Game) collect() {
	for _, x := range coinX {
		if !g.coins[x] && abs(int(g.x)-x) < 4 && abs(int(g.y)-20) < 9 {
			g.coins[x] = true
		}
	}
}
func (g *Game) update() {
	now := time.Now()
	dt := now.Sub(g.last).Seconds()
	if dt > .08 {
		dt = .08
	}
	g.last = now
	if g.over || g.won {
		return
	}
	oldY := g.y
	g.vy += 42 * dt
	g.y += g.vy * dt
	px := int(g.x)
	for _, p := range floors {
		if px+2 >= p.x && px < p.x+p.w && int(oldY)+4 <= p.y && int(g.y)+4 >= p.y {
			g.y = float64(p.y - 4)
			g.vy = 0
		}
	}
	for _, x := range spikes {
		if abs(px-x) < 3 && int(g.y)+4 >= 26 {
			g.over = true
		}
	}
	if g.y > 33 {
		g.over = true
	}
	if g.x > 278 {
		g.won = true
	}
	g.collect()
	target := px - 20
	if target < 0 {
		target = 0
	}
	if target > 220 {
		target = 220
	}
	g.camera = target
}
func (g *Game) Render(c *matrix.Canvas, _ time.Duration) time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.update()
	c.Fill(color.RGBA{5, 12, 35, 255})
	cam := g.camera
	green := color.RGBA{30, 200, 80, 255}
	brown := color.RGBA{100, 55, 20, 255}
	for _, p := range floors {
		x := p.x - cam
		for xx := max(x, 0); xx < min(x+p.w, 64); xx++ {
			c.Set(xx, p.y, green)
			for yy := p.y + 1; yy < 32; yy++ {
				c.Set(xx, yy, brown)
			}
		}
	}
	yellow := color.RGBA{255, 220, 0, 255}
	for _, x := range coinX {
		if !g.coins[x] {
			sx := x - cam
			if sx >= 0 && sx < 64 {
				c.Set(sx, 18, yellow)
				c.Set(sx+1, 19, yellow)
				c.Set(sx, 20, yellow)
			}
		}
	}
	red := color.RGBA{255, 40, 40, 255}
	for _, x := range spikes {
		sx := x - cam
		c.Set(sx, 27, red)
		c.Set(sx+1, 26, red)
		c.Set(sx+2, 27, red)
	}
	px, py := int(g.x)-cam, int(g.y)
	hero := color.RGBA{0, 190, 255, 255}
	for yy := 0; yy < 4; yy++ {
		for xx := 0; xx < 3; xx++ {
			c.Set(px+xx, py+yy, hero)
		}
	}
	white := color.RGBA{255, 255, 255, 255}
	c.DrawText(1, 1, "*"+itoa(len(g.coins)), yellow)
	if g.over {
		c.DrawText(18, 10, "TRY AGAIN", white)
	}
	if g.won {
		c.DrawText(14, 10, "YOU WIN!", yellow)
	}
	return 33 * time.Millisecond
}
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
func itoa(n int) string { return string(rune('0' + n)) }
