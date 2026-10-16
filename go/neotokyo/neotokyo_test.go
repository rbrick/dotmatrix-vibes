package neotokyo

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"dotmatrix/matrix"
)

func TestFetchPrice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"prices":{"usdPrice":"0.297162","etherPrice":"0.000122213272635396"},"totalSupply":"5253722.759727814","marketCap":"1561206.762726237"}`)
	}))
	defer srv.Close()
	p, err := FetchPrice(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if p.USD != 0.297162 || p.ETH != 0.000122213272635396 || p.Supply != 5253722.759727814 || p.MarketCap != 1561206.762726237 {
		t.Errorf("price = %+v", p)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"prices":{"usdPrice":"lots"}}`)
	}))
	defer bad.Close()
	if _, err := FetchPrice(context.Background(), bad.URL); err == nil {
		t.Error("bad number accepted")
	}
}

func TestFormat(t *testing.T) {
	for v, want := range map[float64]string{0.297162: "0.2972", 0.0123456: "0.01235", 1.5: "1.50", 123.456: "123.46", 15234: "15.2K", 2.5e6: "2.50M"} {
		if got := usd(v); got != want {
			t.Errorf("usd(%v) = %q, want %q", v, got, want)
		}
	}
	if got := sig(0.000122213272635396, 3); got != "0.000122" {
		t.Errorf("sig = %q", got)
	}
	if got := compact(1561206.76); got != "1.56M" {
		t.Errorf("compact = %q", got)
	}
}

func TestPriceScene(t *testing.T) {
	if _, err := NewPrice(PriceOptions{Interval: 1}); err == nil {
		t.Error("short interval accepted")
	}
	s, _ := NewPrice(PriceOptions{})
	prices := []float64{0.30, 0.30, 0.31, 0.29}
	var fail bool
	s.fetch = func(context.Context, string) (Price, error) {
		if fail {
			return Price{}, errors.New("down")
		}
		p := Price{USD: prices[0], ETH: 0.0001, MarketCap: 1.5e6}
		prices = prices[1:]
		return p, nil
	}
	c := matrix.NewCanvas()
	for _, want := range []color.RGBA{white, white, upColor, downColor} {
		s.Refresh(context.Background())
		s.Render(c, 0)
		if !has(c, want, 10, 16) {
			t.Errorf("price after %v should be drawn in %v", s.price.USD, want)
		}
	}
	fail = true
	s.Refresh(context.Background())
	if p, err := s.Price(); p == nil || p.USD != 0.29 || err == nil {
		t.Errorf("after an error: %v, %v", p, err)
	}
}

// fakeCitgen serves 64x64 PNGs for odd IDs and 400 for even ones.
func fakeCitgen(t *testing.T) (*httptest.Server, *atomic.Int32) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		var season, w0, h0, id int
		if _, err := fmt.Sscanf(r.URL.Path, "/s%d/%dx%d/%d", &season, &w0, &h0, &id); err != nil || w0 != 64 || h0 != 64 {
			http.Error(w, "bad path", http.StatusNotFound)
			return
		}
		if id%2 == 0 {
			http.Error(w, "execution reverted: ERC721Metadata: URI query for nonexistent token", http.StatusBadRequest)
			return
		}
		img := image.NewRGBA(image.Rect(0, 0, 64, 64))
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 4), uint8(season * 100), 255})
			}
		}
		png.Encode(w, img)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestCitizenSource(t *testing.T) {
	srv, hits := fakeCitgen(t)
	src := &CitizenSource{BaseURL: srv.URL, CacheDir: t.TempDir()}
	ctx := context.Background()
	if _, err := src.Image(ctx, 1, 2); !errors.Is(err, ErrNoCitizen) {
		t.Errorf("missing ID: %v", err)
	}
	for range 2 {
		if img, err := src.Image(ctx, 2, 7); err != nil || img.Bounds().Dx() != 64 {
			t.Fatalf("image: %v", err)
		}
	}
	if hits.Load() != 2 {
		t.Errorf("%d downloads; the second should come from the cache", hits.Load())
	}
	if _, err := os.Stat(filepath.Join(src.CacheDir, "s2", "7.png")); err != nil {
		t.Error(err)
	}
}

func TestCitizenScene(t *testing.T) {
	srv, _ := fakeCitgen(t)
	src := &CitizenSource{BaseURL: srv.URL}
	ctx := context.Background()
	if _, err := NewCitizen(ctx, src, CitizenOptions{Season: 3, ID: 1}); err == nil {
		t.Error("season 3 accepted")
	}
	if _, err := NewCitizen(ctx, src, CitizenOptions{ID: 4}); !errors.Is(err, ErrNoCitizen) {
		t.Errorf("missing citizen: %v", err)
	}

	s, err := NewCitizen(ctx, src, CitizenOptions{ID: 5, Label: true})
	if err != nil {
		t.Fatal(err)
	}
	if season, id := s.Current(); season != 1 || id != 5 {
		t.Errorf("current = S%d #%d", season, id)
	}
	c := matrix.NewCanvas()
	s.Render(c, 0)
	if !has(c, pink, 3, 7) || !has(c, cyan, 24, 28) {
		t.Error("label missing")
	}
	// Whole picture: each pixel averages a 2x2 block of the source.
	if got, want := c.Pix[10][3], (color.RGBA{26, 82, 100, 255}); got != want { // x 6-7, y 20-21
		t.Errorf("pixel = %v, want %v", got, want)
	}
	s.Zoom = true
	s.Render(c, 0)
	if got, want := c.Pix[10][3], (color.RGBA{76, 48, 100, 255}); got != want { // x 16+3, y 2+10
		t.Errorf("zoomed pixel = %v, want %v", got, want)
	}

	// Random: skips missing (even) IDs.
	for range 5 {
		r, err := NewCitizen(ctx, src, CitizenOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if _, id := r.Current(); id%2 == 0 {
			t.Errorf("random picked missing #%d", id)
		}
	}
}

func has(c *matrix.Canvas, col color.RGBA, y0, y1 int) bool {
	for y := y0; y <= y1; y++ {
		for x := range c.Pix[y] {
			if c.Pix[y][x] == col {
				return true
			}
		}
	}
	return false
}
