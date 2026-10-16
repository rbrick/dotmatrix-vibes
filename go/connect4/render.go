package connect4

import (
	"fmt"
	"image/color"
	"time"

	"dotmatrix/matrix"
)

// Layout: a status column on the left, the board on the right. Each cell is
// a 4x4 round hole on a 5-pixel pitch, so the board is 36x31 pixels with a
// one-pixel row above it for the column cursor.
const (
	pitch      = 5
	boardW     = Cols*pitch + 1
	boardH     = Rows*pitch + 1
	boardX     = matrix.Width - boardW
	boardY     = matrix.Height - boardH
	statusW    = boardX - 1
	frameRate  = time.Second / 30
	gravity    = 400.0 // pixels per second², for falling discs
	flashEvery = 250 * time.Millisecond
)

var (
	frameColor = color.RGBA{0, 40, 160, 255}
	discColors = [3]color.RGBA{Red: {255, 0, 0, 255}, Yellow: {255, 190, 0, 255}}
	hintColors = [3]color.RGBA{Red: {70, 0, 0, 255}, Yellow: {70, 50, 0, 255}} // empty holes under the cursor
	white      = color.RGBA{255, 255, 255, 255}
	grey       = color.RGBA{110, 110, 110, 255}
)

// Render implements matrix.Scene.
func (g *Game) Render(c *matrix.Canvas, _ time.Duration) time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	animating := false

	c.Clear()
	for y := boardY; y < boardY+boardH; y++ {
		for x := boardX; x < boardX+boardW; x++ {
			c.Set(x, y, frameColor)
		}
	}

	// A disc still falling is drawn at its current height instead of in
	// its cell.
	var falling *drop
	var fallY int
	if l := g.last; l != nil {
		t := now.Sub(l.at).Seconds()
		startY, endY := -4, boardY+1+l.row*pitch
		if y := startY + int(0.5*gravity*t*t); y < endY {
			falling, fallY, animating = l, y, true
		}
	}

	flashOn := (now.UnixMilli()/flashEvery.Milliseconds())%2 == 0
	humanTurn := !g.over() && !(g.opts.VsComputer && g.turn == Yellow)
	for r := 0; r < Rows; r++ {
		for col := 0; col < Cols; col++ {
			d := g.board[r][col]
			if falling != nil && falling.row == r && falling.col == col {
				d = Empty
			}
			fill := color.RGBA{0, 0, 0, 255}
			switch {
			case d != Empty:
				fill = discColors[d]
				if g.winner != Empty && g.inWinLine(r, col) && falling == nil {
					animating = true
					if !flashOn {
						fill = white
					}
				}
			case humanTurn && col == g.cursor:
				fill = hintColors[g.turn]
			}
			drawDisc(c, boardX+1+col*pitch, boardY+1+r*pitch, fill)
		}
	}
	if falling != nil {
		drawDisc(c, boardX+1+falling.col*pitch, fallY, discColors[falling.disc])
	}
	if humanTurn {
		for x := 0; x < 4; x++ {
			c.Set(boardX+1+g.cursor*pitch+x, 0, discColors[g.turn])
		}
	}

	if g.drawStatus(c, now) {
		animating = true
	}
	if animating {
		return frameRate
	}
	return -1
}

// drawDisc draws a 4x4 disc with its corners cut off, so it looks round.
func drawDisc(c *matrix.Canvas, x, y int, col color.Color) {
	for dy := 0; dy < 4; dy++ {
		for dx := 0; dx < 4; dx++ {
			if (dx == 0 || dx == 3) && (dy == 0 || dy == 3) {
				continue
			}
			c.Set(x+dx, y+dy, col)
		}
	}
}

func (g *Game) inWinLine(r, c int) bool {
	for _, p := range g.winLine {
		if p == [2]int{r, c} {
			return true
		}
	}
	return false
}

func (g *Game) name(d Disc) string {
	switch {
	case g.opts.VsComputer && d == Red:
		return "YOU"
	case g.opts.VsComputer:
		return "CPU"
	case d == Red:
		return "RED"
	}
	return "YEL"
}

// drawStatus fills the left column and reports whether it is animating.
func (g *Game) drawStatus(c *matrix.Canvas, now time.Time) bool {
	center := func(y int, s string, col color.Color) {
		c.DrawText((statusW-matrix.TextWidth(s))/2, y, s, col)
	}
	animating := false
	switch {
	case g.winner != Empty:
		center(3, g.name(g.winner), discColors[g.winner])
		if g.opts.VsComputer && g.winner == Red {
			center(12, "WIN!", white)
		} else {
			center(12, "WINS", white)
		}
	case g.draw:
		center(7, "DRAW", white)
	default:
		center(3, g.name(g.turn), discColors[g.turn])
		if g.thinking {
			// "." ".." "..." while the computer thinks.
			dots := int(now.UnixMilli()/300)%3 + 1
			center(12, "..."[:dots], grey)
			animating = true
		} else {
			drawDisc(c, (statusW-4)/2, 13, discColors[g.turn])
		}
	}

	// Score: red digits, a white dash, yellow digits.
	red, yellow := fmt.Sprint(g.score[Red]), fmt.Sprint(g.score[Yellow])
	w := matrix.TextWidth(red + "-" + yellow)
	x := (statusW - w) / 2
	x = c.DrawText(x, 23, red, discColors[Red])
	x = c.DrawText(x, 23, "-", grey)
	c.DrawText(x, 23, yellow, discColors[Yellow])
	return animating
}
