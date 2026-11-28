package main

import (
	"context"
	"encoding/json"
	"image/color"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"dotmatrix/matrix"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connectMCP serves the MCP handler for a panel-less Player and connects a
// client to it over HTTP, as an agent would.
func connectMCP(t *testing.T) (*mcp.ClientSession, *matrix.Player, *int) {
	t.Helper()
	p := newPlayer(t)
	saves := 0
	srv := httptest.NewServer(newMCPHandler(p, func() { saves++ }))
	t.Cleanup(srv.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: srv.URL, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs, p, &saves
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// waitFrame waits until the Player has rendered its current scene.
func waitFrame(t *testing.T, p *matrix.Player, check func(*matrix.Canvas) bool) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if f, _ := p.Frame(); check(&f) {
			return
		}
	}
	t.Fatal("panel never showed the expected frame")
}

func TestMCPTools(t *testing.T) {
	cs, _, _ := connectMCP(t)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "draw,read_pixels,view_panel" {
		t.Errorf("tools = %s", got)
	}
}

func TestMCPDrawAndRead(t *testing.T) {
	cs, p, saves := connectMCP(t)
	res := call(t, cs, "draw", map[string]any{"base": "blank", "ops": []map[string]any{
		{"op": "rect", "x": 0, "y": 0, "w": 64, "h": 32, "color": "blue"},
		{"op": "line", "x": 2, "y": 2, "x2": 6, "y2": 2, "color": "#ff0000"},
		{"op": "circle", "x": 20, "y": 15, "r": 3, "filled": true, "color": "yellow"},
		{"op": "fill", "x": 40, "y": 20, "color": "green"},
		{"op": "sprite", "x": 50, "y": 4, "rows": []string{"p.p", ".p."}, "palette": map[string]string{"p": "pink"}},
		{"op": "pixel", "x": 10, "y": 10, "points": [][]int{{11, 10}}, "color": "white"},
	}})
	if res.IsError {
		t.Fatalf("draw failed: %v", res.Content)
	}
	if len(res.Content) != 2 {
		t.Fatalf("draw should return text and a picture, got %d items", len(res.Content))
	}
	if img, ok := res.Content[1].(*mcp.ImageContent); !ok || img.MIMEType != "image/png" || len(img.Data) == 0 {
		t.Errorf("second item should be a PNG, got %T", res.Content[1])
	}
	if *saves != 1 {
		t.Errorf("draw should save the state once, saved %d times", *saves)
	}
	blue, red, green := color.RGBA{0, 0, 255, 255}, color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}
	yellow, white := color.RGBA{255, 255, 0, 255}, color.RGBA{255, 255, 255, 255}
	pink, _ := matrix.ParseColor("pink")
	waitFrame(t, p, func(c *matrix.Canvas) bool { return c.Pix[0][0] == blue })
	f, _ := p.Frame()
	for _, c := range []struct {
		x, y int
		want color.RGBA
	}{
		{0, 31, blue}, {63, 0, blue}, // rect outline
		{2, 2, red}, {6, 2, red}, {7, 2, green}, // line ends, then the fill around it
		{20, 15, yellow}, {23, 15, yellow}, {20, 12, yellow}, {24, 15, green}, // circle
		{50, 4, pink}, {51, 4, green}, {51, 5, pink}, // sprite: '.' is transparent
		{10, 10, white}, {11, 10, white},
	} {
		if got := f.Pix[c.y][c.x]; got != c.want {
			t.Errorf("pixel %d,%d = %v, want %v", c.x, c.y, got, c.want)
		}
	}

	// read_pixels returns the sprite format; reading back what was drawn.
	res = call(t, cs, "read_pixels", map[string]any{"x": 49, "y": 4, "w": 4, "h": 2})
	var out pixelsOutput
	b, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 2 || len(out.Rows[0]) != 4 {
		t.Fatalf("rows = %q", out.Rows)
	}
	lookup := func(x, y int) string { return out.Palette[string(out.Rows[y][x])] }
	if lookup(1, 0) != matrix.HexColor(pink) || lookup(0, 0) != "#00ff00" || lookup(2, 1) != matrix.HexColor(pink) {
		t.Errorf("read back %q with palette %v", out.Rows, out.Palette)
	}
}

func TestMCPDrawsOnCurrentPicture(t *testing.T) {
	cs, p, _ := connectMCP(t)
	call(t, cs, "draw", map[string]any{"base": "blank", "ops": []map[string]any{{"op": "clear", "color": "red"}}})
	red := color.RGBA{255, 0, 0, 255}
	waitFrame(t, p, func(c *matrix.Canvas) bool { return c.Pix[5][5] == red })
	call(t, cs, "draw", map[string]any{"ops": []map[string]any{{"op": "pixel", "x": 0, "y": 0, "color": "blue"}}})
	waitFrame(t, p, func(c *matrix.Canvas) bool { return c.Pix[0][0] == color.RGBA{0, 0, 255, 255} })
	if f, _ := p.Frame(); f.Pix[5][5] != red {
		t.Error("drawing on the current picture should keep what was there")
	}
}

func TestMCPDrawErrors(t *testing.T) {
	cs, _, saves := connectMCP(t)
	for _, ops := range [][]map[string]any{
		{{"op": "spray"}},
		{{"op": "pixel", "color": "not-a-colour"}},
		{{"op": "rect", "w": 0, "h": 3}},
		{{"op": "text", "text": "hi", "font": "huge"}},
		{{"op": "pixel", "points": [][]int{{1}}}},
	} {
		if res := call(t, cs, "draw", map[string]any{"ops": ops}); !res.IsError {
			t.Errorf("ops %v should fail", ops)
		}
	}
	if res := call(t, cs, "read_pixels", map[string]any{"x": 60, "w": 10}); !res.IsError {
		t.Error("a region off the panel should fail")
	}
	if *saves != 0 {
		t.Error("failed draws must not change the panel")
	}
}

func TestMCPViewPanel(t *testing.T) {
	cs, _, _ := connectMCP(t)
	res := call(t, cs, "view_panel", map[string]any{})
	if res.IsError || len(res.Content) != 2 {
		t.Fatalf("view_panel = %v", res.Content)
	}
	if text := res.Content[0].(*mcp.TextContent).Text; !strings.Contains(text, "not connected") {
		t.Errorf("should say the panel isn't connected: %q", text)
	}
}

func TestCircleShapes(t *testing.T) {
	c := matrix.NewCanvas()
	drawCircle(c, 10, 10, 0, false, color.White)
	n := 0
	for y := range c.Pix {
		for x := range c.Pix[y] {
			if c.Pix[y][x] != (color.RGBA{}) {
				n++
			}
		}
	}
	if n != 1 {
		t.Errorf("r=0 circle lit %d pixels, want 1", n)
	}
	c = matrix.NewCanvas()
	drawCircle(c, 10, 10, 4, false, color.White)
	if c.Pix[10][10] != (color.RGBA{}) {
		t.Error("an outline circle should be hollow")
	}
	if c.Pix[10][14] == (color.RGBA{}) || c.Pix[6][10] == (color.RGBA{}) {
		t.Error("outline should reach the radius")
	}
}
