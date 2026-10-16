// Package connect4 is a Connect Four game that renders onto the LED panel
// (it implements matrix.Scene), with an optional computer opponent.
package connect4

import (
	"errors"
	"sync"
	"time"
)

const (
	Cols = 7
	Rows = 6
)

// Disc is the content of a board cell, and also identifies a player.
type Disc int8

const (
	Empty Disc = iota
	Red
	Yellow
)

func (d Disc) other() Disc { return 3 - d }

func (d Disc) String() string {
	switch d {
	case Red:
		return "red"
	case Yellow:
		return "yellow"
	}
	return ""
}

// Board is indexed [row][col]; row 0 is the top.
type Board [Rows][Cols]Disc

// Difficulty is how many moves ahead the computer looks.
type Difficulty string

const (
	Easy   Difficulty = "easy"
	Normal Difficulty = "normal"
	Hard   Difficulty = "hard"
)

func (d Difficulty) depth() int {
	switch d {
	case Easy:
		return 2
	case Hard:
		return 8
	}
	return 5
}

// Options configures a new game.
type Options struct {
	VsComputer bool
	Difficulty Difficulty // default Normal
	CPUFirst   bool       // the computer (yellow) moves first
}

// Game is safe for concurrent use.
type Game struct {
	mu       sync.Mutex
	onChange func()
	opts     Options
	board    Board
	turn     Disc
	cursor   int
	winner   Disc
	winLine  [4][2]int // [row, col] of the winning discs
	draw     bool
	thinking bool
	score    [3]int // indexed by Disc
	last     *drop
	gen      int // incremented by Reset, so stale computer moves are dropped
}

type drop struct {
	row, col int
	disc     Disc
	at       time.Time
}

// SetOnChange registers f to be called after the computer moves by itself,
// so the display can be redrawn.
func (g *Game) SetOnChange(f func()) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.onChange = f
}

// New starts a game.
func New(opts Options) *Game {
	g := &Game{}
	g.Reset(opts)
	return g
}

// Reset starts a new round, keeping the score.
func (g *Game) Reset(opts Options) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if opts.Difficulty == "" {
		opts.Difficulty = Normal
	}
	g.opts = opts
	g.board = Board{}
	g.turn, g.cursor, g.winner, g.draw, g.thinking, g.last = Red, Cols/2, Empty, false, false, nil
	g.gen++
	if opts.VsComputer && opts.CPUFirst {
		g.turn = Yellow
		g.scheduleComputer()
	}
}

var (
	ErrGameOver    = errors.New("connect4: the game is over")
	ErrNotYourTurn = errors.New("connect4: it is the computer's turn")
	ErrColumnFull  = errors.New("connect4: that column is full")
	ErrBadColumn   = errors.New("connect4: column must be 0-6")
)

// Drop drops the current player's disc into col. Against the computer it
// may only be used on the human's (red's) turn.
func (g *Game) Drop(col int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	switch {
	case col < 0 || col >= Cols:
		return ErrBadColumn
	case g.over():
		return ErrGameOver
	case g.opts.VsComputer && g.turn == Yellow:
		return ErrNotYourTurn
	}
	if err := g.play(col); err != nil {
		return err
	}
	if g.opts.VsComputer && !g.over() {
		g.scheduleComputer()
	}
	return nil
}

// DropAtCursor drops into the column the cursor is on.
func (g *Game) DropAtCursor() error {
	g.mu.Lock()
	col := g.cursor
	g.mu.Unlock()
	return g.Drop(col)
}

// MoveCursor moves the column cursor by delta, wrapping around.
func (g *Game) MoveCursor(delta int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cursor = ((g.cursor+delta)%Cols + Cols) % Cols
}

// SetCursor puts the cursor on col.
func (g *Game) SetCursor(col int) error {
	if col < 0 || col >= Cols {
		return ErrBadColumn
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cursor = col
	return nil
}

func (g *Game) over() bool { return g.winner != Empty || g.draw }

// play drops a disc for g.turn and updates the result; g.mu must be held.
func (g *Game) play(col int) error {
	row := g.board.dropRow(col)
	if row < 0 {
		return ErrColumnFull
	}
	g.board[row][col] = g.turn
	g.last = &drop{row: row, col: col, disc: g.turn, at: time.Now()}
	if line, ok := g.board.winAt(row, col); ok {
		g.winner, g.winLine = g.turn, line
		g.score[g.turn]++
	} else if g.board.full() {
		g.draw = true
	} else {
		g.turn = g.turn.other()
	}
	return nil
}

// computerDelay lets the human's disc land before the computer replies.
const computerDelay = 700 * time.Millisecond

// scheduleComputer makes the computer move soon; g.mu must be held.
func (g *Game) scheduleComputer() {
	g.thinking = true
	gen, board, depth := g.gen, g.board, g.opts.Difficulty.depth()
	go func() {
		start := time.Now()
		col := bestMove(board, Yellow, depth)
		time.Sleep(computerDelay - time.Since(start))

		g.mu.Lock()
		if gen != g.gen { // a new game started meanwhile
			g.mu.Unlock()
			return
		}
		g.thinking = false
		g.play(col)
		g.cursor = col
		onChange := g.onChange
		g.mu.Unlock()
		if onChange != nil {
			onChange()
		}
	}()
}

// dropRow returns the row a disc dropped into col lands in, or -1 if full.
func (b *Board) dropRow(col int) int {
	for row := Rows - 1; row >= 0; row-- {
		if b[row][col] == Empty {
			return row
		}
	}
	return -1
}

func (b *Board) full() bool {
	for col := 0; col < Cols; col++ {
		if b[0][col] == Empty {
			return false
		}
	}
	return true
}

var directions = [4][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}}

// winAt reports whether the disc at (row, col) is part of four in a row.
func (b *Board) winAt(row, col int) ([4][2]int, bool) {
	d := b[row][col]
	for _, dir := range directions {
		// Walk back to the start of the run through (row, col), then forward.
		r, c := row, col
		for in(r-dir[0], c-dir[1]) && b[r-dir[0]][c-dir[1]] == d {
			r, c = r-dir[0], c-dir[1]
		}
		var line [4][2]int
		n := 0
		for in(r, c) && b[r][c] == d {
			if n < 4 {
				line[n] = [2]int{r, c}
			}
			n++
			r, c = r+dir[0], c+dir[1]
		}
		if n >= 4 {
			return line, true
		}
	}
	return [4][2]int{}, false
}

func in(r, c int) bool { return r >= 0 && r < Rows && c >= 0 && c < Cols }

// State is a JSON-friendly snapshot of a game.
type State struct {
	Board      [Rows][Cols]string `json:"board"` // "", "red" or "yellow"
	Turn       string             `json:"turn"`
	Cursor     int                `json:"cursor"`
	Winner     string             `json:"winner,omitempty"`
	WinLine    [][2]int           `json:"winLine,omitempty"`
	Draw       bool               `json:"draw"`
	Thinking   bool               `json:"thinking"`
	VsComputer bool               `json:"vsComputer"`
	Difficulty Difficulty         `json:"difficulty"`
	CPUFirst   bool               `json:"cpuFirst"`
	Score      map[string]int     `json:"score"`
}

func (g *Game) State() State {
	g.mu.Lock()
	defer g.mu.Unlock()
	s := State{
		Turn: g.turn.String(), Cursor: g.cursor, Winner: g.winner.String(), Draw: g.draw,
		Thinking: g.thinking, VsComputer: g.opts.VsComputer, Difficulty: g.opts.Difficulty,
		CPUFirst: g.opts.CPUFirst, Score: map[string]int{"red": g.score[Red], "yellow": g.score[Yellow]},
	}
	for r := range g.board {
		for c, d := range g.board[r] {
			s.Board[r][c] = d.String()
		}
	}
	if g.winner != Empty {
		s.WinLine = g.winLine[:]
	}
	return s
}

// Restore recreates a game from a State, e.g. one saved before a restart.
func Restore(s State) (*Game, error) {
	parse := func(v string) (Disc, error) {
		switch v {
		case "":
			return Empty, nil
		case "red":
			return Red, nil
		case "yellow":
			return Yellow, nil
		}
		return Empty, errors.New("connect4: bad disc " + v)
	}
	g := New(Options{VsComputer: s.VsComputer, Difficulty: s.Difficulty})
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gen++ // cancel anything Reset scheduled
	g.thinking = false
	g.opts.CPUFirst = s.CPUFirst
	for r := range s.Board {
		for c, v := range s.Board[r] {
			d, err := parse(v)
			if err != nil {
				return nil, err
			}
			g.board[r][c] = d
		}
	}
	var err error
	if g.turn, err = parse(s.Turn); err != nil || g.turn == Empty {
		return nil, errors.New("connect4: bad turn")
	}
	if s.Cursor >= 0 && s.Cursor < Cols {
		g.cursor = s.Cursor
	}
	g.score[Red], g.score[Yellow] = s.Score["red"], s.Score["yellow"]
	g.draw = s.Draw
	if g.winner, err = parse(s.Winner); err != nil {
		return nil, err
	}
	if g.winner != Empty && len(s.WinLine) == 4 {
		copy(g.winLine[:], s.WinLine)
	}
	if g.opts.VsComputer && g.turn == Yellow && !g.over() {
		g.scheduleComputer()
	}
	return g, nil
}
