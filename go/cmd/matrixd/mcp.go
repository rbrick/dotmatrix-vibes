package main

// An MCP (Model Context Protocol) server at /mcp lets AI agents look at the
// panel and draw on it, like the control page's Paint tab:
//
//	claude mcp add --transport http dotmatrix http://dotmatrix.local:8080/mcp

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"
	"sync"

	"dotmatrix/matrix"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpInstructions = `This server drives a 64×32 RGB LED matrix panel. (0,0) is the top-left
pixel, x grows to the right (0-63) and y downwards (0-31).

Use view_panel to see what it shows, then draw with draw: one call takes a
list of operations (pixels, lines, rectangles, circles, flood fills, text and
palette sprites) and shows the result, starting from what the panel shows
(base "current") or from black (base "blank"). Drawing replaces whatever was
on the panel (text, a scene, a game) with a still picture, as the control
page's Paint tab does. read_pixels returns exact colours of a region.

Colours are "#rrggbb", "#rgb" or a name (red, green, blue, yellow, cyan,
magenta, orange, purple, pink, white, black). The LEDs show 16 levels per
channel through a gamma curve, so very dark colours (below about #202020)
look black and subtle shades merge; bold, saturated colours work best.
Text in the normal font is 7 px tall and 6 px per character; the small
font is 5 px tall and 4 px per character (capitals only). Normal text can
contain emoji (7×7 sprites) and inline colour codes like :pink:.`

// drawOp is one drawing operation. Which fields matter depends on Op.
type drawOp struct {
	Op      string            `json:"op" jsonschema:"one of: pixel, line, rect, circle, fill, text, sprite, clear"`
	X       int               `json:"x,omitempty" jsonschema:"pixel/rect/text/sprite: left x; line: start x; circle: centre x; fill: seed x"`
	Y       int               `json:"y,omitempty" jsonschema:"pixel/rect/text/sprite: top y; line: start y; circle: centre y; fill: seed y"`
	X2      int               `json:"x2,omitempty" jsonschema:"line: end x"`
	Y2      int               `json:"y2,omitempty" jsonschema:"line: end y"`
	W       int               `json:"w,omitempty" jsonschema:"rect: width"`
	H       int               `json:"h,omitempty" jsonschema:"rect: height"`
	R       int               `json:"r,omitempty" jsonschema:"circle: radius (0 is a single pixel)"`
	Color   string            `json:"color,omitempty" jsonschema:"colour for pixel, line, rect, circle, fill, text and clear (default white; clear defaults to black)"`
	Filled  bool              `json:"filled,omitempty" jsonschema:"rect/circle: fill the shape instead of drawing its outline"`
	Points  [][]int           `json:"points,omitempty" jsonschema:"pixel: extra [x,y] pairs to set in the same colour"`
	Text    string            `json:"text,omitempty" jsonschema:"text: one line; may contain emoji and inline colour codes such as :pink:"`
	Font    string            `json:"font,omitempty" jsonschema:"text: normal (5×7, default) or small (3×5 capitals)"`
	Rows    []string          `json:"rows,omitempty" jsonschema:"sprite: rows of characters, top to bottom; each character is looked up in palette"`
	Palette map[string]string `json:"palette,omitempty" jsonschema:"sprite: character → colour; characters not in it (and '.' or ' ' unless listed) are transparent"`
}

type drawInput struct {
	Base string   `json:"base,omitempty" jsonschema:"current (default): draw on top of what the panel shows; blank: start from black"`
	Ops  []drawOp `json:"ops" jsonschema:"operations, applied in order"`
}

type viewInput struct{}

type regionInput struct {
	X int `json:"x,omitempty" jsonschema:"left x (default 0)"`
	Y int `json:"y,omitempty" jsonschema:"top y (default 0)"`
	W int `json:"w,omitempty" jsonschema:"width (default to the right edge)"`
	H int `json:"h,omitempty" jsonschema:"height (default to the bottom edge)"`
}

type pixelsOutput struct {
	X       int               `json:"x"`
	Y       int               `json:"y"`
	Rows    []string          `json:"rows" jsonschema:"one string per row; each character is a colour in palette"`
	Palette map[string]string `json:"palette" jsonschema:"character → #rrggbb; '.' is black"`
}

// newMCPHandler serves the drawing tools. Drawing replaces the panel's
// content with a still picture, saved like any other image.
func newMCPHandler(player *matrix.Player, persist func()) http.Handler {
	server := mcp.NewServer(&mcp.Implementation{Name: "dotmatrix", Version: "1.0.0"},
		&mcp.ServerOptions{Instructions: mcpInstructions})
	var drawing sync.Mutex // one draw at a time, so concurrent agents don't lose strokes

	mcp.AddTool(server, &mcp.Tool{
		Name:        "view_panel",
		Description: "Show what the 64×32 LED panel is displaying now, as a picture: each LED is an 8×8 block, with brighter grid lines every 8 LEDs (x = 0, 8, 16 … and y = 0, 8, 16, 24) to help count positions.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ viewInput) (*mcp.CallToolResult, any, error) {
		frame, _ := player.Frame()
		return panelResult(&frame, describeState(player))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "read_pixels",
		Description: "Return the exact colours of a rectangle of the panel (the whole panel by default) as rows of palette characters, the same format the draw tool's sprite operation takes.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in regionInput) (*mcp.CallToolResult, pixelsOutput, error) {
		frame, _ := player.Frame()
		out, err := readPixels(&frame, in)
		return nil, out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "draw",
		Description: "Draw on the LED panel and show the result. Operations run in order on a 64×32 canvas that starts as what the panel shows (or black with base \"blank\"); off-panel pixels are clipped. " +
			"Operations: pixel {x,y,color,points}; line {x,y,x2,y2,color}; rect {x,y,w,h,color,filled}; circle {x,y,r,color,filled}; fill {x,y,color} (flood-fills the area of the seed pixel's colour); " +
			"text {x,y,text,color,font}; sprite {x,y,rows,palette} (pixel art, e.g. rows [\".rr.\",\"rrrr\"] with palette {\"r\":\"red\"}); clear {color}. " +
			"Returns a picture of the panel afterwards.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: false, DestructiveHint: ptr(true)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in drawInput) (*mcp.CallToolResult, any, error) {
		drawing.Lock()
		defer drawing.Unlock()
		c := matrix.NewCanvas()
		switch in.Base {
		case "", "current":
			frame, _ := player.Frame()
			*c = frame
		case "blank":
		default:
			return nil, nil, fmt.Errorf("base must be current or blank, not %q", in.Base)
		}
		for i, op := range in.Ops {
			if err := applyOp(c, op); err != nil {
				return nil, nil, fmt.Errorf("ops[%d] (%s): %w", i, op.Op, err)
			}
		}
		player.Show(matrix.Still{Canvas: c})
		persist()
		return panelResult(c, fmt.Sprintf("Drew %d operation(s); the panel now shows this picture.", len(in.Ops)))
	})

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	// Keep web pages in a browser from drawing through the user's session.
	return http.NewCrossOriginProtection().Handler(handler)
}

func ptr[T any](v T) *T { return &v }

func describeState(player *matrix.Player) string {
	s := player.State()
	conn := "connected"
	if !s.Connected {
		conn = "not connected (the picture is what it would show)"
	}
	return fmt.Sprintf("The panel is %s, showing %q mode at brightness %d/255.", conn, snapshot(player).Mode, s.Brightness)
}

// panelResult returns a picture of c with a line of text.
func panelResult(c *matrix.Canvas, text string) (*mcp.CallToolResult, any, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, panelImage(c)); err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.TextContent{Text: text},
		&mcp.ImageContent{Data: buf.Bytes(), MIMEType: "image/png"},
	}}, nil, nil
}

// panelImage draws each LED as a 7×7 block on an 8×8 grid, with brighter
// grid lines every 8 LEDs, so an agent can count positions.
func panelImage(c *matrix.Canvas) image.Image {
	const scale = 8
	img := image.NewRGBA(image.Rect(0, 0, matrix.Width*scale+1, matrix.Height*scale+1))
	grid, major := color.RGBA{28, 28, 28, 255}, color.RGBA{80, 80, 80, 255}
	for y := 0; y <= matrix.Height*scale; y++ {
		for x := 0; x <= matrix.Width*scale; x++ {
			switch {
			case x%(8*scale) == 0 || y%(8*scale) == 0:
				img.Set(x, y, major)
			case x%scale == 0 || y%scale == 0:
				img.Set(x, y, grid)
			default:
				img.Set(x, y, c.Pix[y/scale][x/scale])
			}
		}
	}
	return img
}

const paletteChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!#$%&*+-/<=>?@^~"

func readPixels(c *matrix.Canvas, in regionInput) (pixelsOutput, error) {
	if in.W == 0 {
		in.W = matrix.Width - in.X
	}
	if in.H == 0 {
		in.H = matrix.Height - in.Y
	}
	if in.X < 0 || in.Y < 0 || in.W <= 0 || in.H <= 0 || in.X+in.W > matrix.Width || in.Y+in.H > matrix.Height {
		return pixelsOutput{}, fmt.Errorf("region %d,%d %d×%d is not inside the 64×32 panel", in.X, in.Y, in.W, in.H)
	}
	out := pixelsOutput{X: in.X, Y: in.Y, Palette: map[string]string{".": "#000000"}}
	chars := map[color.RGBA]byte{{0, 0, 0, 255}: '.'}
	for y := in.Y; y < in.Y+in.H; y++ {
		row := make([]byte, in.W)
		for x := in.X; x < in.X+in.W; x++ {
			p := c.Pix[y][x]
			p.A = 255
			ch, ok := chars[p]
			if !ok {
				if len(chars)-1 >= len(paletteChars) {
					return pixelsOutput{}, fmt.Errorf("region has more than %d colours; read a smaller one", len(paletteChars)+1)
				}
				ch = paletteChars[len(chars)-1]
				chars[p] = ch
				out.Palette[string(ch)] = matrix.HexColor(p)
			}
			row[x-in.X] = ch
		}
		out.Rows = append(out.Rows, string(row))
	}
	return out, nil
}

func parseColorOr(s string, def color.RGBA) (color.RGBA, error) {
	if s == "" {
		return def, nil
	}
	return matrix.ParseColor(s)
}

func applyOp(c *matrix.Canvas, op drawOp) error {
	def := color.RGBA{255, 255, 255, 255}
	if op.Op == "clear" {
		def = color.RGBA{0, 0, 0, 255}
	}
	col, err := parseColorOr(op.Color, def)
	if err != nil {
		return err
	}
	switch op.Op {
	case "pixel":
		c.Set(op.X, op.Y, col)
		for _, p := range op.Points {
			if len(p) != 2 {
				return fmt.Errorf("points must be [x,y] pairs")
			}
			c.Set(p[0], p[1], col)
		}
	case "line":
		drawLine(c, op.X, op.Y, op.X2, op.Y2, col)
	case "rect":
		if op.W <= 0 || op.H <= 0 {
			return fmt.Errorf("w and h must be positive")
		}
		for y := op.Y; y < op.Y+op.H; y++ {
			for x := op.X; x < op.X+op.W; x++ {
				if op.Filled || x == op.X || y == op.Y || x == op.X+op.W-1 || y == op.Y+op.H-1 {
					c.Set(x, y, col)
				}
			}
		}
	case "circle":
		if op.R < 0 {
			return fmt.Errorf("r must not be negative")
		}
		drawCircle(c, op.X, op.Y, op.R, op.Filled, col)
	case "fill":
		floodFill(c, op.X, op.Y, col)
	case "text":
		switch op.Font {
		case "", "normal":
			c.DrawText(op.X, op.Y, op.Text, col)
		case "small":
			c.DrawSmallText(op.X, op.Y, op.Text, col)
		default:
			return fmt.Errorf("font must be normal or small, not %q", op.Font)
		}
	case "sprite":
		pal := map[rune]*color.RGBA{}
		for k, v := range op.Palette {
			r := []rune(k)
			if len(r) != 1 {
				return fmt.Errorf("palette keys must be single characters, not %q", k)
			}
			if v == "" || strings.EqualFold(v, "transparent") {
				pal[r[0]] = nil
				continue
			}
			pc, err := matrix.ParseColor(v)
			if err != nil {
				return fmt.Errorf("palette %q: %w", k, err)
			}
			pal[r[0]] = &pc
		}
		for dy, row := range op.Rows {
			for dx, ch := range []rune(row) {
				if pc := pal[ch]; pc != nil {
					c.Set(op.X+dx, op.Y+dy, *pc)
				}
			}
		}
	case "clear":
		c.Fill(col)
	default:
		return fmt.Errorf("unknown op %q (want pixel, line, rect, circle, fill, text, sprite or clear)", op.Op)
	}
	return nil
}

// drawLine draws a straight line with Bresenham's algorithm.
func drawLine(c *matrix.Canvas, x0, y0, x1, y1 int, col color.Color) {
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := sign(x1-x0), sign(y1-y0)
	e := dx + dy
	for {
		c.Set(x0, y0, col)
		if x0 == x1 && y0 == y1 {
			return
		}
		if 2*e >= dy {
			e += dy
			x0 += sx
		}
		if 2*e <= dx {
			e += dx
			y0 += sy
		}
	}
}

// drawCircle draws the pixels within r of (cx, cy), or just the edge of
// that disc. r*r+r instead of r*r rounds the shape nicely at small sizes.
func drawCircle(c *matrix.Canvas, cx, cy, r int, filled bool, col color.Color) {
	inside := func(dx, dy int) bool { return dx*dx+dy*dy <= r*r+r }
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if !inside(dx, dy) {
				continue
			}
			edge := !inside(dx+1, dy) || !inside(dx-1, dy) || !inside(dx, dy+1) || !inside(dx, dy-1)
			if filled || edge {
				c.Set(cx+dx, cy+dy, col)
			}
		}
	}
}

// floodFill paints the 4-connected area of the seed pixel's colour.
func floodFill(c *matrix.Canvas, x, y int, col color.Color) {
	if x < 0 || x >= matrix.Width || y < 0 || y >= matrix.Height {
		return
	}
	target := c.Pix[y][x]
	repl := color.RGBAModel.Convert(col).(color.RGBA)
	if target == repl {
		return
	}
	stack := [][2]int{{x, y}}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		px, py := p[0], p[1]
		if px < 0 || px >= matrix.Width || py < 0 || py >= matrix.Height || c.Pix[py][px] != target {
			continue
		}
		c.Pix[py][px] = repl
		stack = append(stack, [2]int{px + 1, py}, [2]int{px - 1, py}, [2]int{px, py + 1}, [2]int{px, py - 1})
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func sign(v int) int {
	switch {
	case v < 0:
		return -1
	case v > 0:
		return 1
	}
	return 0
}
