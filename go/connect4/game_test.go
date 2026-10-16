package connect4

import (
	"testing"
	"time"

	"dotmatrix/matrix"
)

// play drops discs alternately starting with red, ignoring the computer.
func playCols(t *testing.T, g *Game, cols ...int) {
	t.Helper()
	for _, c := range cols {
		g.mu.Lock()
		err := g.play(c)
		g.mu.Unlock()
		if err != nil {
			t.Fatalf("play(%d): %v", c, err)
		}
	}
}

func TestWinDirections(t *testing.T) {
	for name, tc := range map[string]struct {
		moves []int
		line  [4][2]int
	}{
		"horizontal":  {[]int{0, 0, 1, 1, 2, 2, 3}, [4][2]int{{5, 0}, {5, 1}, {5, 2}, {5, 3}}},
		"vertical":    {[]int{6, 5, 6, 5, 6, 5, 6}, [4][2]int{{2, 6}, {3, 6}, {4, 6}, {5, 6}}},
		"diagonal /":  {[]int{0, 1, 1, 2, 2, 3, 2, 3, 3, 6, 3}, [4][2]int{{2, 3}, {3, 2}, {4, 1}, {5, 0}}},
		"diagonal \\": {[]int{6, 5, 5, 4, 4, 3, 4, 3, 3, 0, 3}, [4][2]int{{2, 3}, {3, 4}, {4, 5}, {5, 6}}},
	} {
		t.Run(name, func(t *testing.T) {
			g := New(Options{})
			playCols(t, g, tc.moves...)
			s := g.State()
			if s.Winner != "red" {
				t.Fatalf("winner = %q, want red", s.Winner)
			}
			for _, p := range tc.line {
				if !g.inWinLine(p[0], p[1]) {
					t.Errorf("win line %v missing %v", g.winLine, p)
				}
			}
			if s.Score["red"] != 1 {
				t.Errorf("score = %v", s.Score)
			}
			if err := g.Drop(4); err != ErrGameOver {
				t.Errorf("Drop after win: %v", err)
			}
		})
	}
}

func TestDrawAndFullColumn(t *testing.T) {
	g := New(Options{})
	playCols(t, g, 0, 0, 0, 0, 0, 0)
	if err := g.Drop(0); err != ErrColumnFull {
		t.Errorf("Drop on full column: %v", err)
	}
	// Fill the whole board in a pattern that never makes four in a row.
	g = New(Options{})
	order := []int{0, 1, 0, 1, 0, 1, 1, 0, 1, 0, 1, 0, 2, 3, 2, 3, 2, 3, 3, 2, 3, 2, 3, 2, 4, 5, 4, 5, 4, 5, 5, 4, 5, 4, 5, 4, 6, 6, 6, 6, 6, 6}
	playCols(t, g, order...)
	if s := g.State(); !s.Draw || s.Winner != "" {
		t.Fatalf("expected a draw, got winner %q draw %v", s.Winner, s.Draw)
	}
}

func TestComputerWinsAndBlocks(t *testing.T) {
	var b Board
	// Yellow has three in a row on the bottom; it should complete it.
	b[5][0], b[5][1], b[5][2] = Yellow, Yellow, Yellow
	b[4][0], b[4][1], b[4][2] = Red, Red, Red
	if got := bestMove(b, Yellow, Easy.depth()); got != 3 {
		t.Errorf("computer should win in column 3, chose %d", got)
	}
	// Red threatens three in a row vertically in column 6; yellow must block.
	b = Board{}
	b[5][6], b[4][6], b[3][6] = Red, Red, Red
	b[5][0], b[5][1] = Yellow, Yellow
	if got := bestMove(b, Yellow, Normal.depth()); got != 6 {
		t.Errorf("computer should block in column 6, chose %d", got)
	}
}

func TestHardIsFastEnough(t *testing.T) {
	start := time.Now()
	bestMove(Board{}, Yellow, Hard.depth())
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("hard move on an empty board took %v", d)
	}
}

func TestComputerReplies(t *testing.T) {
	changed := make(chan struct{}, 1)
	g := New(Options{VsComputer: true, Difficulty: Easy})
	g.SetOnChange(func() { changed <- struct{}{} })
	if err := g.Drop(3); err != nil {
		t.Fatal(err)
	}
	if err := g.Drop(2); err != ErrNotYourTurn {
		t.Errorf("Drop during computer's turn: %v", err)
	}
	select {
	case <-changed:
	case <-time.After(3 * time.Second):
		t.Fatal("computer never moved")
	}
	if s := g.State(); s.Turn != "red" || s.Thinking {
		t.Errorf("after computer move: turn %s thinking %v", s.Turn, s.Thinking)
	}
}

func TestStateRestore(t *testing.T) {
	g := New(Options{})
	playCols(t, g, 3, 3, 4)
	g.MoveCursor(-1)
	r, err := Restore(g.State())
	if err != nil {
		t.Fatal(err)
	}
	if a, b := g.State(), r.State(); a.Board != b.Board || a.Turn != b.Turn || a.Cursor != b.Cursor {
		t.Errorf("restored %+v, want %+v", b, a)
	}
}

func TestRenderFits(t *testing.T) {
	g := New(Options{VsComputer: true})
	c := matrix.NewCanvas()
	g.Render(c, 0)
	if boardX < 0 || boardY < 1 || statusW < matrix.TextWidth("WINS") {
		t.Errorf("layout doesn't fit: boardX %d boardY %d statusW %d", boardX, boardY, statusW)
	}
}
