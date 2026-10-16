package matrix

import (
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
	if !text.scrolls() {
		t.Fatal("long text should scroll in auto mode")
	}
	period := time.Duration(Width+text.width()) * text.step()
	a, b := NewCanvas(), NewCanvas()
	text.Render(a, time.Second)
	text.Render(b, time.Second+period)
	if *a != *b {
		t.Error("scroll position should repeat after one full period")
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
