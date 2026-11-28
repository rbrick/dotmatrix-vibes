package matrix

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"
)

func TestEncodeWireFormat(t *testing.T) {
	c := NewCanvas()
	c.Set(0, 0, color.RGBA{255, 0, 0, 255})
	c.Set(1, 0, color.RGBA{0, 255, 255, 255})
	c.Set(Width-1, Height-1, color.RGBA{0, 0, 255, 255})
	buf := make([]byte, Width*Height*2)
	c.encode(buf)
	if got := uint16(buf[0]) | uint16(buf[1])<<8; got != 0xf00 {
		t.Errorf("red pixel = %#x, want 0xf00", got)
	}
	if got := uint16(buf[2]) | uint16(buf[3])<<8; got != 0x0ff {
		t.Errorf("cyan pixel = %#x, want 0x0ff", got)
	}
	if got := uint16(buf[len(buf)-2]) | uint16(buf[len(buf)-1])<<8; got != 0x00f {
		t.Errorf("last pixel = %#x, want 0x00f", got)
	}
}

func TestTextLayout(t *testing.T) {
	if w := TextWidth("HELLO"); w != 29 {
		t.Errorf("TextWidth = %d, want 29", w)
	}
	c := NewCanvas()
	Text{Text: "I", Color: color.White}.Render(c, 0)
	// 'I' has its stem in glyph column 2; centred, it lands at x=(64-5)/2+2.
	if c.Pix[(Height-7)/2][(Width-5)/2+2] != (color.RGBA{255, 255, 255, 255}) {
		t.Error("centred text not where expected")
	}
}

func TestScrollWrapsAround(t *testing.T) {
	text := Text{Text: "a message much wider than sixty-four pixels"}.WithDefaults()
	w := TextWidth(text.Text)
	if !text.scrolls(w) {
		t.Fatal("long text should scroll in auto mode")
	}
	period := time.Duration(Width+w) * text.step()
	a, b := NewCanvas(), NewCanvas()
	text.Render(a, time.Second)
	text.Render(b, time.Second+period)
	if *a != *b {
		t.Error("scroll position should repeat after one full period")
	}
}

// litColumns returns the x of every lit pixel in rows [y0, y1).
func litColumns(c *Canvas, y0, y1 int) map[int]bool {
	xs := map[int]bool{}
	for y := y0; y < y1; y++ {
		for x := 0; x < Width; x++ {
			if c.Pix[y][x] != (color.RGBA{0, 0, 0, 255}) {
				xs[x] = true
			}
		}
	}
	return xs
}

func TestEachLineScrollsOnItsOwn(t *testing.T) {
	text := Text{Text: "HI\na message much wider than sixty-four pixels"}.WithDefaults()
	a, b := NewCanvas(), NewCanvas()
	text.Render(a, 0)
	next := text.Render(b, 2*time.Second)
	if next < 0 {
		t.Fatal("text with a long line should keep animating")
	}
	// Lines are 7 px tall with a 1 px gap, centred: rows 8..15 and 16..23.
	top := (Height - 15) / 2
	if fmt.Sprint(litColumns(a, top, top+7)) != fmt.Sprint(litColumns(b, top, top+7)) {
		t.Error("the short line should stay centred while the long one scrolls")
	}
	if fmt.Sprint(litColumns(a, top+8, top+15)) == fmt.Sprint(litColumns(b, top+8, top+15)) {
		t.Error("the long line should scroll")
	}
	if x := litColumns(a, top, top+7); !x[(Width-TextWidth("HI"))/2] {
		t.Error("short line not centred")
	}
}

func TestScrollAlwaysMovesEveryLine(t *testing.T) {
	text := Text{Text: "A\nBBBBBB", Scroll: ScrollAlways}.WithDefaults()
	a, b := NewCanvas(), NewCanvas()
	text.Render(a, 0)
	text.Render(b, time.Second)
	top := (Height - 15) / 2
	for _, y := range []int{top, top + 8} {
		if fmt.Sprint(litColumns(a, y, y+7)) == fmt.Sprint(litColumns(b, y, y+7)) {
			t.Errorf("line at y=%d should scroll", y)
		}
	}
}

func TestInlineColours(t *testing.T) {
	pink := namedColors["pink"]
	green := color.RGBA{0, 255, 0, 255}
	colours := func(s string) []color.Color {
		cs := cells(s, nil)
		var out []color.Color
		for _, c := range cs {
			out = append(out, c.col)
		}
		return out
	}
	got := colours(":pink:AB:#0f0:C:default:D")
	want := []color.Color{pink, pink, green, nil}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("colours = %v, want %v", got, want)
	}
	// Codes take no space; hex needs '#', so :add: stays text.
	if TextWidth(":pink:test") != TextWidth("test") {
		t.Error("colour code should take no width")
	}
	if TextWidth(":add:") != TextWidth("add")+2*(TextWidth(":")+1) {
		t.Error(":add: should be drawn as text")
	}
	// Image shortcodes are offered colour names first, so images win clashes.
	var seen []string
	ScanShortcodes(":pink:hi :pikachu:", func(name string) bool { seen = append(seen, name); return false })
	if fmt.Sprint(seen) != "[pink pikachu]" {
		t.Errorf("scanner offered %v", seen)
	}

	// Drawn: a code colours the rest of its own line only, so the lines
	// after it go back to the text's colour.
	c := NewCanvas()
	Text{Text: "I\n:pink:I\nI", Color: color.White, Align: AlignLeft}.Render(c, 0)
	top := (Height - 23) / 2
	for i, want := range []color.RGBA{{255, 255, 255, 255}, pink, {255, 255, 255, 255}} {
		if got := c.Pix[top+8*i][2]; got != want {
			t.Errorf("line %d drawn in %v, want %v", i+1, got, want)
		}
	}
}

func TestTypographicPunctuation(t *testing.T) {
	for in, want := range map[string]string{
		"don’t": "don't",
		"“Hi”":  `"Hi"`,
		"a—b–c": "a-b-c",
		"wait…": "wait...",
		"12 pm": "12 pm",
	} {
		a, b := NewCanvas(), NewCanvas()
		a.DrawText(0, 0, in, color.White)
		b.DrawText(0, 0, want, color.White)
		if *a != *b || TextWidth(in) != TextWidth(want) {
			t.Errorf("%q should draw like %q", in, want)
		}
	}
}

func TestParseColor(t *testing.T) {
	for in, want := range map[string]color.RGBA{
		"#ff8000": {255, 128, 0, 255}, "0f0": {0, 255, 0, 255}, "Red": {255, 0, 0, 255},
	} {
		if got, err := ParseColor(in); err != nil || got != want {
			t.Errorf("ParseColor(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseColor("nope"); err == nil {
		t.Error("expected error for invalid colour")
	}
}

func TestEmoji(t *testing.T) {
	red := color.RGBA{255, 0, 0, 255}
	for _, s := range []string{"❤", "❤️", ":heart:", "♥"} {
		if w := TextWidth(s); w != emojiWidth {
			t.Errorf("TextWidth(%q) = %d, want %d", s, w, emojiWidth)
		}
		c := NewCanvas()
		c.DrawText(0, 0, s, color.White)
		if c.Pix[3][3] != red {
			t.Errorf("%q: heart centre = %v, want red", s, c.Pix[3][3])
		}
	}
	// Text and emoji mix; unknown shortcodes stay as text.
	if w, want := TextWidth("I :heart: GO"), TextWidth("I X GO")+emojiWidth-glyphWidth; w != want {
		t.Errorf("mixed width = %d, want %d", w, want)
	}
	if w, want := TextWidth(":nope:"), TextWidth("nope")+2*(TextWidth(":")+1); w != want {
		t.Errorf("unknown shortcode width = %d, want %d", w, want)
	}
	for _, e := range emojis {
		if len(e.art) != glyphHeight {
			t.Errorf("%s: %d rows", e.name, len(e.art))
		}
		for _, row := range e.art {
			if len(row) != emojiWidth {
				t.Errorf("%s: row %q is not %d wide", e.name, row, emojiWidth)
			}
			for i := 0; i < len(row); i++ {
				if _, ok := emojiPalette[row[i]]; !ok && row[i] != '.' {
					t.Errorf("%s: unknown palette char %q", e.name, row[i])
				}
			}
		}
	}
}

func TestNarrowPunctuation(t *testing.T) {
	if w := TextWidth(":"); w != 2 {
		t.Errorf("colon width = %d, want 2", w)
	}
	if w := TextWidth("12:59"); w != 4*6+3-1 {
		t.Errorf("12:59 width = %d", w)
	}
	c := NewCanvas()
	c.DrawText(0, 0, ":", color.White)
	if c.Pix[1][0] == (color.RGBA{}) || c.Pix[1][1] == (color.RGBA{}) {
		t.Error("trimmed colon should start at x=0")
	}
}

func TestShortcodesNeverContainSpaces(t *testing.T) {
	// The ":30 :" between a time and an emoji must not be taken as a code.
	if w, want := TextWidth("12:30 :heart:"), TextWidth("12:30 ")+emojiWidth+1; w != want {
		t.Errorf("width = %d, want %d", w, want)
	}
	var seen []string
	ScanShortcodes("at 12:30:45 :x: a: b:", func(name string) bool { seen = append(seen, name); return false })
	for _, n := range seen {
		if strings.ContainsRune(n, ' ') {
			t.Errorf("scanner offered %q", n)
		}
	}
}

func TestQuantizeKeepsHue(t *testing.T) {
	for _, c := range []struct {
		in   color.RGBA
		want uint16
	}{
		{color.RGBA{255, 255, 255, 255}, 0xfff},
		{color.RGBA{255, 0, 0, 255}, 0xf00},
		{color.RGBA{0, 0, 0, 255}, 0x000},
		{color.RGBA{55, 54, 53, 255}, 0x111}, // dim grey: no channel rounds differently
		{color.RGBA{60, 30, 0, 255}, 0x100},  // too dim for orange's green: red only
		{color.RGBA{8, 8, 8, 255}, 0x000},    // too dim to show at all
	} {
		if got := Quantize(c.in); got != c.want {
			t.Errorf("Quantize(%v) = %03x, want %03x", c.in, got, c.want)
		}
	}
	// A grey fading out stays grey at every step.
	for v := 0; v < 256; v++ {
		q := Quantize(color.RGBA{uint8(v), uint8(v * 98 / 100), uint8(v * 96 / 100), 255})
		if r, g, b := q>>8, q>>4&15, q&15; r-g > 1 || r-b > 1 || (r < 4 && (g != r || b != r)) {
			t.Errorf("warm grey %d quantized to %03x", v, q)
		}
	}
}

func TestEmojiPixels(t *testing.T) {
	for _, e := range Emojis() {
		if len(e.Pixels) != 7 || len(e.Pixels[0]) != emojiWidth {
			t.Fatalf("%s: sprite is %d rows", e.Name, len(e.Pixels))
		}
		if e.Name == "heart" {
			if got := e.Pixels[0]; got[0] != "" || got[1] != "#ff0000" {
				t.Errorf("heart top row = %q, want transparent then red", got)
			}
		}
	}
}
