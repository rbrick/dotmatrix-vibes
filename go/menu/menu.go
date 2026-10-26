// Package menu renders controller-operated menus on the LED matrix.
package menu

import (
	"image/color"
	"sync"
	"time"

	"dotmatrix/matrix"
)

// Item is one selectable menu entry. ID is interpreted by the caller.
type Item struct {
	ID    string
	Label string
}

// Menu is a thread-safe matrix scene.
type Menu struct {
	mu       sync.Mutex
	Title    string
	Items    []Item
	selected int
}

func New(title string, items []Item) *Menu {
	return &Menu{Title: title, Items: append([]Item(nil), items...)}
}

// Move changes the selection, wrapping at either end.
func (m *Menu) Move(delta int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Items) > 0 {
		m.selected = ((m.selected+delta)%len(m.Items) + len(m.Items)) % len(m.Items)
	}
}

// Current returns the selected item.
func (m *Menu) Current() (Item, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Items) == 0 {
		return Item{}, false
	}
	return m.Items[m.selected], true
}

func (m *Menu) Render(c *matrix.Canvas, _ time.Duration) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	c.Clear()
	white := color.RGBA{255, 255, 255, 255}
	cyan := color.RGBA{0, 220, 255, 255}
	yellow := color.RGBA{255, 210, 0, 255}
	c.DrawText((matrix.Width-matrix.TextWidth(m.Title))/2, 0, m.Title, cyan)
	if len(m.Items) == 0 {
		c.DrawText(4, 13, "NO ITEMS", white)
		return -1
	}
	// Three rows fit below the title. Keep the selection in the middle where possible.
	start := m.selected - 1
	if start < 0 {
		start = 0
	}
	if start > len(m.Items)-3 {
		start = len(m.Items) - 3
	}
	if start < 0 {
		start = 0
	}
	for row, i := 0, start; i < len(m.Items) && row < 3; row, i = row+1, i+1 {
		prefix, col := " ", white
		if i == m.selected {
			prefix, col = ">", yellow
		}
		label := m.Items[i].Label
		for matrix.TextWidth(prefix+label) > matrix.Width && len(label) > 1 {
			label = label[:len(label)-1]
		}
		c.DrawText(0, 9+row*8, prefix+label, col)
	}
	return -1
}
