package main

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"dotmatrix/matrix"
)

var plain = options{Width: 1, Gamma: 1, Saturation: 1}

// halves returns a w×h picture, red on the left half and blue on the right.
func halves(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{255, 0, 0, 255}
			if x >= w/2 {
				c = color.RGBA{0, 0, 255, 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func TestCropAveragesEachLED(t *testing.T) {
	c, err := convert(halves(640, 320), plain)
	if err != nil {
		t.Fatal(err)
	}
	if c.Pix[10][0] != (color.RGBA{255, 0, 0, 255}) || c.Pix[10][63] != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("edges = %v, %v", c.Pix[10][0], c.Pix[10][63])
	}
	// A picture 2:1 at 3×3 picture pixels per LED, the LED on the boundary
	// straddles both colours. (192 px / 64 = 3, boundary at 96.)
	c, _ = convert(halves(190, 95), plain)
	if c.Pix[0][31] == c.Pix[0][32] {
		t.Error("the two halves should differ")
	}
}

func TestCropMustFit(t *testing.T) {
	// A 2:1 crop of a square picture is half its height: top 0.6 runs off.
	_, err := convert(halves(100, 100), options{Top: 0.6, Width: 1, Gamma: 1, Saturation: 1})
	if err == nil || !strings.Contains(err.Error(), "at most 0.500") {
		t.Errorf("err = %v", err)
	}
	if _, err := convert(halves(100, 100), options{Top: 0.5, Width: 1, Gamma: 1, Saturation: 1}); err != nil {
		t.Errorf("top 0.5 should fit: %v", err)
	}
	if _, err := convert(halves(100, 100), options{Left: 0.5, Width: 0.6, Gamma: 1, Saturation: 1}); err == nil {
		t.Error("a crop past the right edge should fail")
	}
}

func TestFitCentresAndContinuesEdges(t *testing.T) {
	// A square picture fits as 32×32 in the middle; the sides repeat its edges.
	o := plain
	o.Fit = true
	c, err := convert(halves(100, 100), o)
	if err != nil {
		t.Fatal(err)
	}
	red, blue := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}
	for _, p := range []struct {
		x    int
		want color.RGBA
	}{{0, red}, {16, red}, {31, red}, {32, blue}, {47, blue}, {63, blue}} {
		if got := c.Pix[16][p.x]; got != p.want {
			t.Errorf("x=%d: %v, want %v", p.x, got, p.want)
		}
	}
	// A very wide picture fits the width and repeats its top and bottom rows.
	c, err = convert(halves(400, 50), o)
	if err != nil {
		t.Fatal(err)
	}
	if c.Pix[0][0] != red || c.Pix[31][63] != blue {
		t.Error("wide picture should fill the panel")
	}
}

func TestAdjust(t *testing.T) {
	if got := adjust(64, 64, 64, options{Gamma: 0.5, Saturation: 1}); got.R != 128 {
		t.Errorf("gamma 0.5 should lift 64 to 128, got %d", got.R)
	}
	if got := adjust(200, 100, 100, options{Gamma: 1, Saturation: 0}); got.R != got.G || got.G != got.B {
		t.Errorf("saturation 0 should be grey, got %v", got)
	}
}

func TestPreviewSize(t *testing.T) {
	if b := ledPreview(matrix.NewCanvas()).Bounds(); b.Dx() != 512 || b.Dy() != 256 {
		t.Errorf("preview is %v", b)
	}
}
