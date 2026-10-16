package pokemon

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dotmatrix/matrix"
)

func TestLookup(t *testing.T) {
	for q, want := range map[string]string{
		"pikachu": "Pikachu", "PIKACHU": "Pikachu", "25": "Pikachu", "#025": "Pikachu",
		"mr mime": "Mr. Mime", "Mr. Mime": "Mr. Mime", "nidoran-f": "Nidoran♀", "nidoranf": "Nidoran♀",
		"flabebe": "Flabébé", "type: null": "Type: Null", "905": "Enamorus",
	} {
		if e, err := Lookup(q); err != nil || e.Name != want {
			t.Errorf("Lookup(%q) = %v, %v; want %s", q, e.Name, err, want)
		}
	}
	for _, q := range []string{"0", "906", "agumon", ""} {
		if _, err := Lookup(q); err == nil {
			t.Errorf("Lookup(%q) should fail", q)
		}
	}
	if len(Pokedex) != 905 || Pokedex[0].Name != "Bulbasaur" {
		t.Errorf("Pokedex has %d entries starting with %v", len(Pokedex), Pokedex[0])
	}
}

func TestLabelFitsSmallFont(t *testing.T) {
	for _, e := range Pokedex {
		if w := matrix.SmallTextWidth(e.Label()); w > matrix.Width-2 {
			t.Errorf("%s label is %dpx; wider than the panel", e.Name, w)
		}
	}
}

// spriteServer serves a w x h opaque red block inside a 68x56 transparent
// box sprite, and counts requests.
func spriteServer(t *testing.T, w, h int) (*httptest.Server, *atomic.Int32) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if !strings.HasSuffix(r.URL.Path, ".png") {
			http.NotFound(rw, r)
			return
		}
		img := image.NewNRGBA(image.Rect(0, 0, 68, 56))
		for y := 10; y < 10+h; y++ {
			for x := 5; x < 5+w; x++ {
				img.Set(x, y, color.NRGBA{255, 0, 0, 255})
			}
		}
		var buf bytes.Buffer
		png.Encode(&buf, img)
		rw.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestSpriteCroppedCachedAndFitted(t *testing.T) {
	srv, hits := spriteServer(t, 20, 18)
	src := &Source{BaseURL: srv.URL, CacheDir: t.TempDir(), Client: srv.Client()}
	s, err := New(context.Background(), src, Options{Pokemon: "pikachu", Label: true})
	if err != nil {
		t.Fatal(err)
	}
	if b := s.cur.sprite.Bounds(); b.Dx() != 20 || b.Dy() != 18 {
		t.Errorf("small sprite should be cropped, not scaled: %v", b)
	}
	if _, err := os.Stat(filepath.Join(src.CacheDir, "regular", "pikachu.png")); err != nil {
		t.Errorf("sprite not cached: %v", err)
	}
	New(context.Background(), src, Options{Pokemon: "25"})
	if hits.Load() != 1 {
		t.Errorf("second load should come from the cache; %d downloads", hits.Load())
	}
	c := matrix.NewCanvas()
	s.Render(c, 0)
	if c.Pix[16][32] != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("centre pixel = %v, want the red sprite", c.Pix[16][32])
	}

	big, _ := spriteServer(t, 58, 49)
	src = &Source{BaseURL: big.URL, Client: big.Client()}
	s, err = New(context.Background(), src, Options{Pokemon: "eternatus", Shiny: true})
	if err != nil {
		t.Fatal(err)
	}
	if b := s.cur.sprite.Bounds(); b.Dy() != matrix.Height || b.Dx() > matrix.Width {
		t.Errorf("large sprite should be scaled to fit: %v", b)
	}
}

func TestErrors(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	src := &Source{BaseURL: srv.URL, Client: srv.Client()}
	if _, err := New(context.Background(), src, Options{Pokemon: "pikachu"}); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("missing sprite: err = %v", err)
	}
	if _, err := New(context.Background(), src, Options{Pokemon: "agumon"}); err == nil {
		t.Error("unknown Pokémon should fail")
	}
}

func TestRandomRotates(t *testing.T) {
	srv, _ := spriteServer(t, 10, 10)
	src := &Source{BaseURL: srv.URL, Client: srv.Client()}
	s, err := New(context.Background(), src, Options{Pokemon: "random", Dwell: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	c := matrix.NewCanvas()
	s.Render(c, 0) // starts prefetching the next one
	deadline := time.Now().Add(3 * time.Second)
	for {
		s.mu.Lock()
		ready := s.next != nil
		s.mu.Unlock()
		if ready || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.Render(c, 1500*time.Millisecond)
	if s.since != 1.5 {
		t.Errorf("should have switched to the prefetched Pokémon at 1.5s (since = %v)", s.since)
	}
}

func TestInlineImages(t *testing.T) {
	srv, hits := spriteServer(t, 20, 18)
	src := &Source{BaseURL: srv.URL, Client: srv.Client()}
	text := "I :heart: :pikachu: at 12:30:45 :Mr-Mime: :shiny-pikachu: :agumon: :pikachu:"
	images, err := InlineImages(context.Background(), src, text)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pikachu", "Mr-Mime", "shiny-pikachu"} {
		if images[name] == nil {
			t.Errorf("missing image for :%s:", name)
		}
	}
	for _, name := range []string{"heart", "30", "agumon"} {
		if images[name] != nil {
			t.Errorf(":%s: should not be a Pokémon", name)
		}
	}
	if hits.Load() != 3 { // pikachu, mr-mime, shiny pikachu; the repeat is reused
		t.Errorf("%d downloads, want 3", hits.Load())
	}

	// Drawn in a line of text: the line grows to the sprite's height and the
	// letters sit in its middle.
	c := matrix.NewCanvas()
	tx := matrix.Text{Text: "HI :pikachu:", Images: images, Scroll: matrix.ScrollNever}
	if next := tx.Render(c, 0); next >= 0 {
		t.Error("text with a sprite should be static")
	}
	if h := matrix.TextHeight("HI :pikachu:", images); h != 18 {
		t.Errorf("line height = %d, want 18", h)
	}
	x0 := (matrix.Width - matrix.TextWidthImages("HI :pikachu:", images)) / 2
	spriteX := x0 + matrix.TextWidth("HI ") + 1
	if c.Pix[16][spriteX+5] != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("sprite pixel = %v, want red", c.Pix[16][spriteX+5])
	}
}

func TestInlineImagesReportsFetchErrors(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	src := &Source{BaseURL: srv.URL, Client: srv.Client()}
	if _, err := InlineImages(context.Background(), src, "hello :pikachu:"); err == nil {
		t.Error("expected an error when the sprite can't be downloaded")
	}
}

func TestBalls(t *testing.T) {
	if len(Balls) != 38 {
		t.Errorf("%d balls, want PokéSprite's 38", len(Balls))
	}
	for q, want := range map[string]string{
		"pokeball": "poke", "Poké Ball": "poke", "poke-ball": "poke", "master": "master",
		"hisuian-pokeball": "hisuian-poke", "Hisuian Great Ball": "hisuian-great",
	} {
		if b, err := LookupBall(q); err != nil || b.Slug != want {
			t.Errorf("LookupBall(%q) = %v, %v; want %s", q, b, err, want)
		}
	}
	if _, err := LookupBall("pikachu"); err == nil {
		t.Error("pikachu is not a ball")
	}
	if b, _ := LookupBall("hisuian-poke"); b.Name() != "Hisuian Poké Ball" || b.Shortcode() != "hisuian-pokeball" {
		t.Errorf("name %q, shortcode %q", b.Name(), b.Shortcode())
	}
}

func TestInlineBalls(t *testing.T) {
	var paths []string
	var mu sync.Mutex
	srv, _ := spriteServer(t, 18, 18)
	inner := srv.Config.Handler
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		inner.ServeHTTP(w, r)
	})
	src := &Source{BaseURL: srv.URL, Client: srv.Client()}
	images, err := InlineImages(context.Background(), src, ":pokeball: :MasterBall: :moon: :pikachu:")
	if err != nil {
		t.Fatal(err)
	}
	if images["pokeball"] == nil || images["MasterBall"] == nil || images["moon"] != nil {
		t.Errorf("images: %v", images)
	}
	sort.Strings(paths)
	want := []string{"/items/ball/master.png", "/items/ball/poke.png", "/pokemon-gen8/regular/pikachu.png"}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("downloaded %v, want %v", paths, want)
	}
}
