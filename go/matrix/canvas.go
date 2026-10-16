package matrix

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"
	"strings"
)

// Panel dimensions in pixels.
const (
	Width  = 64
	Height = 32
)

// Canvas is an off-screen 64x32 image. It implements draw.Image, so the
// standard image/draw package can paint onto it.
type Canvas struct {
	Pix [Height][Width]color.RGBA
}

var _ draw.Image = (*Canvas)(nil)

func NewCanvas() *Canvas { return &Canvas{} }

func (c *Canvas) ColorModel() color.Model { return color.RGBAModel }
func (c *Canvas) Bounds() image.Rectangle { return image.Rect(0, 0, Width, Height) }

func (c *Canvas) At(x, y int) color.Color {
	if x < 0 || x >= Width || y < 0 || y >= Height {
		return color.RGBA{}
	}
	return c.Pix[y][x]
}

// Set paints one pixel; coordinates outside the panel are ignored.
func (c *Canvas) Set(x, y int, col color.Color) {
	if x < 0 || x >= Width || y < 0 || y >= Height {
		return
	}
	c.Pix[y][x] = color.RGBAModel.Convert(col).(color.RGBA)
}

// Fill paints every pixel.
func (c *Canvas) Fill(col color.Color) {
	rgba := color.RGBAModel.Convert(col).(color.RGBA)
	for y := range c.Pix {
		for x := range c.Pix[y] {
			c.Pix[y][x] = rgba
		}
	}
}

// Clear sets every pixel to black.
func (c *Canvas) Clear() { c.Pix = [Height][Width]color.RGBA{} }

// DrawText draws a single line of text with its top-left corner at (x, y) and
// returns the x coordinate just past the last character. Emoji characters and
// :shortcodes: (see Emojis) are drawn as colour sprites; other characters
// outside printable ASCII are drawn as '?'.
func (c *Canvas) DrawText(x, y int, s string, col color.Color) int {
	return c.DrawTextImages(x, y, s, col, nil)
}

// DrawTextImages is DrawText with pictures for extra :shortcodes:. The line
// is TextHeight tall with (x, y) its top-left corner; letters and images are
// centred vertically within it.
func (c *Canvas) DrawTextImages(x, y int, s string, col color.Color, images Images) int {
	cs := cells(s, images)
	h := lineHeight(cs)
	for _, cl := range cs {
		top := y + (h-cl.height())/2
		switch {
		case cl.emoji != nil:
			cl.emoji.draw(c, x, top)
		case cl.img != nil:
			c.blend(x, top, cl.img)
		default:
			for gx := 0; gx < cl.width; gx++ {
				for gy := 0; gy < glyphHeight; gy++ {
					if cl.glyph[cl.first+gx]>>gy&1 != 0 {
						c.Set(x+gx, top+gy, col)
					}
				}
			}
		}
		x += cl.advance()
	}
	return x
}

// blend draws img with its top-left corner at (x, y), respecting its alpha.
func (c *Canvas) blend(x, y int, img image.Image) {
	b := img.Bounds()
	for sy := b.Min.Y; sy < b.Max.Y; sy++ {
		for sx := b.Min.X; sx < b.Max.X; sx++ {
			dx, dy := x+sx-b.Min.X, y+sy-b.Min.Y
			if dx < 0 || dx >= Width || dy < 0 || dy >= Height {
				continue
			}
			r, g, bl, a := img.At(sx, sy).RGBA() // premultiplied, 16-bit
			if a == 0 {
				continue
			}
			bg := c.Pix[dy][dx]
			keep := 0xffff - a
			mixc := func(src uint32, dst uint8) uint8 { return uint8((src + uint32(dst)*0x101*keep/0xffff) >> 8) }
			c.Pix[dy][dx] = color.RGBA{mixc(r, bg.R), mixc(g, bg.G), mixc(bl, bg.B), 255}
		}
	}
}

func lineHeight(cs []cell) int {
	h := glyphHeight
	for _, cl := range cs {
		h = max(h, cl.height())
	}
	return h
}

// TextHeight returns how tall a line of text is: the font height, or the
// tallest image in it.
func TextHeight(s string, images Images) int { return lineHeight(cells(s, images)) }

// DrawImage scales img down (nearest neighbour, preserving aspect ratio) to
// fit the panel if necessary, then draws it centred.
func (c *Canvas) DrawImage(img image.Image) {
	b := img.Bounds()
	scale := math.Min(1, math.Min(float64(Width)/float64(b.Dx()), float64(Height)/float64(b.Dy())))
	w, h := int(float64(b.Dx())*scale), int(float64(b.Dy())*scale)
	ox, oy := (Width-w)/2, (Height-h)/2
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			src := img.At(b.Min.X+int(float64(x)/scale), b.Min.Y+int(float64(y)/scale))
			draw.Draw(c, image.Rect(ox+x, oy+y, ox+x+1, oy+y+1), &image.Uniform{src}, image.Point{}, draw.Over)
		}
	}
}

// TextWidth returns the width in pixels of a single line of text, without the
// trailing gap after the last character.
func TextWidth(s string) int { return TextWidthImages(s, nil) }

// TextWidthImages is TextWidth with pictures for extra :shortcodes:.
func TextWidthImages(s string, images Images) int {
	w := 0
	for _, cl := range cells(s, images) {
		w += cl.advance()
	}
	return max(w-1, 0)
}

// encode converts the canvas to the firmware's wire format: little-endian
// 0x0RGB with 4 bits per channel, gamma corrected so that colours look
// roughly like they do on a monitor.
func (c *Canvas) encode(buf []byte) {
	i := 0
	for y := range c.Pix {
		for x := range c.Pix[y] {
			v := Quantize(c.Pix[y][x])
			buf[i], buf[i+1] = byte(v), byte(v>>8)
			i += 2
		}
	}
}

// Quantize returns the panel's 0x0RGB value (0-15 per channel) for a colour.
// The brightest channel is rounded to the nearest level and the others are
// scaled in proportion, so a colour keeps its hue at every brightness: a
// dim grey stays grey instead of some channels rounding up and others down.
func Quantize(p color.RGBA) uint16 {
	m := max(p.R, p.G, p.B)
	top := uint16(math.Round(linear[m]))
	if top == 0 {
		// Too dim to show. Rather than going black, light the dominant
		// channels at the lowest level, so dim colours stay visible and keep
		// their hue (a dim orange stays red, not grey or yellow).
		if m < 16 {
			return 0
		}
		dominant := func(v uint8) uint16 {
			if int(v)*2 >= int(m) {
				return 1
			}
			return 0
		}
		return dominant(p.R)<<8 | dominant(p.G)<<4 | dominant(p.B)
	}
	level := func(v uint8) uint16 { return uint16(math.Round(linear[v] / linear[m] * float64(top))) }
	return level(p.R)<<8 | level(p.G)<<4 | level(p.B)
}

// linear is the gamma-corrected brightness of each 8-bit value on the
// panel's 0-15 scale.
var linear = func() (t [256]float64) {
	for i := range t {
		t[i] = 15 * math.Pow(float64(i)/255, 2.2)
	}
	return
}()

var namedColors = map[string]color.RGBA{
	"black":   {0, 0, 0, 255},
	"white":   {255, 255, 255, 255},
	"red":     {255, 0, 0, 255},
	"green":   {0, 255, 0, 255},
	"blue":    {0, 0, 255, 255},
	"yellow":  {255, 255, 0, 255},
	"cyan":    {0, 255, 255, 255},
	"magenta": {255, 0, 255, 255},
	"orange":  {255, 128, 0, 255},
	"purple":  {128, 0, 255, 255},
	"pink":    {255, 64, 128, 255},
}

// ParseColor accepts "#rrggbb", "rrggbb", "#rgb" or a basic colour name.
func ParseColor(s string) (color.RGBA, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if c, ok := namedColors[s]; ok {
		return c, nil
	}
	hex := strings.TrimPrefix(s, "#")
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return color.RGBA{}, fmt.Errorf("invalid colour %q", s)
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("invalid colour %q", s)
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, nil
}

// HexColor formats a colour as "#rrggbb".
func HexColor(c color.Color) string {
	r := color.RGBAModel.Convert(c).(color.RGBA)
	return fmt.Sprintf("#%02x%02x%02x", r.R, r.G, r.B)
}
