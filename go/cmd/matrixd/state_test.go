package main

import (
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"dotmatrix/connect4"
	"dotmatrix/matrix"
	"dotmatrix/minecraft"
	"dotmatrix/moonphase"
	"dotmatrix/neotokyo"
	"dotmatrix/orrery"
	"dotmatrix/planets"
	"dotmatrix/pokemon"
	"dotmatrix/prayerclock"
	"dotmatrix/starfront"
)

// newPlayer returns a Player that never finds a Pico; only its state matters.
func newPlayer(t *testing.T) *matrix.Player {
	p := matrix.NewPlayer(filepath.Join(t.TempDir(), "no-such-port"))
	t.Cleanup(p.Close)
	return p
}

func TestStateRoundTrip(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, matrix.Width, matrix.Height))
	img.Set(3, 4, color.RGBA{255, 0, 0, 255})
	clock, err := prayerclock.New(41.8781, -87.6298, "MWL", true)
	if err != nil {
		t.Fatal(err)
	}

	for name, show := range map[string]func(*matrix.Player){
		"text": func(p *matrix.Player) {
			p.ShowText(matrix.Text{Text: "HI :heart:", Color: color.RGBA{255, 128, 0, 255}, Scroll: matrix.ScrollNever, Speed: 40})
		},
		"prayer": func(p *matrix.Player) { p.Show(clock) },
		"planet": func(p *matrix.Player) {
			s, _ := planets.New(planets.Options{Body: "saturn", Label: true, Speed: 1.5})
			p.Show(s)
		},
		"image": func(p *matrix.Player) { p.ShowImage(img) },
		"moon":  func(p *matrix.Player) { p.Show(moonphase.New(true)) },
		"minecraft": func(p *matrix.Player) {
			s, err := minecraft.New(minecraft.Options{Address: "mc.example.com:25570", Icon: minecraft.IconServer})
			if err != nil {
				t.Fatal(err)
			}
			p.Show(s)
		},
		"starfront": func(p *matrix.Player) {
			s, err := starfront.New(starfront.Options{Building: 3, Interval: 20 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			p.Show(s)
		},
		"bytes": func(p *matrix.Player) {
			s, err := neotokyo.NewPrice(neotokyo.PriceOptions{Interval: time.Minute})
			if err != nil {
				t.Fatal(err)
			}
			p.Show(s)
		},
		"solarsystem": func(p *matrix.Player) { p.Show(orrery.New(orrery.Options{Speed: 7, Label: true})) },
		"rotation": func(p *matrix.Player) {
			req := defaultRotation(-33.87, 151.21, "MWL", false)
			req.Items = append(req.Items, rotationItem{sceneSpec{Mode: "text", Text: &textRequest{Text: "HI"}}, 5})
			r, err := req.scene()
			if err != nil {
				t.Fatal(err)
			}
			p.Show(r)
		},
		"connect4": func(p *matrix.Player) {
			g := connect4.New(connect4.Options{})
			g.Drop(3)
			g.Drop(4)
			g.MoveCursor(2)
			p.Show(g)
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := &stateFile{path: filepath.Join(t.TempDir(), "sub", "state.json")}
			before := newPlayer(t)
			show(before)
			before.SetBrightness(42)
			want := snapshot(before)
			if err := f.save(want); err != nil {
				t.Fatal(err)
			}

			loaded, err := f.load()
			if err != nil {
				t.Fatal(err)
			}
			after := newPlayer(t)
			if err := loaded.apply(after, func(*connect4.Game) {}); err != nil {
				t.Fatal(err)
			}
			if got := snapshot(after); !reflect.DeepEqual(got, want) {
				t.Errorf("restored state differs:\n got %+v\nwant %+v", got, want)
			}
			if got := after.State().Brightness; got != 42 {
				t.Errorf("brightness = %d, want 42", got)
			}
		})
	}
}

func TestPokemonStateRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		img := image.NewNRGBA(image.Rect(0, 0, 68, 56))
		img.Set(30, 30, color.NRGBA{255, 255, 0, 255})
		png.Encode(w, img)
	}))
	defer srv.Close()
	saved := spriteSource
	spriteSource = &pokemon.Source{BaseURL: srv.URL, Client: srv.Client()}
	defer func() { spriteSource = saved }()

	f := &stateFile{path: filepath.Join(t.TempDir(), "state.json")}
	before := newPlayer(t)
	s, err := pokemonRequest{Pokemon: "25", Shiny: true, Label: true}.scene()
	if err != nil {
		t.Fatal(err)
	}
	before.Show(s)
	want := snapshot(before)
	if want.Pokemon.Pokemon != "Pikachu" {
		t.Errorf("saved name = %q, want the resolved name Pikachu", want.Pokemon.Pokemon)
	}
	f.save(want)
	loaded, _ := f.load()
	after := newPlayer(t)
	if err := loaded.apply(after, nil); err != nil {
		t.Fatal(err)
	}
	if got := snapshot(after); !reflect.DeepEqual(got, want) {
		t.Errorf("restored %+v, want %+v", got.Pokemon, want.Pokemon)
	}
}

func TestStateLoadMissingAndCorrupt(t *testing.T) {
	dir := t.TempDir()
	if _, err := (&stateFile{path: filepath.Join(dir, "none.json")}).load(); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file: err = %v, want ErrNotExist", err)
	}
	if _, err := (&stateFile{}).load(); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("disabled state: err = %v, want ErrNotExist", err)
	}
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte("{not json"), 0o644)
	if _, err := (&stateFile{path: bad}).load(); err == nil {
		t.Error("corrupt file should fail to load")
	}
	if err := (content{sceneSpec: sceneSpec{Mode: "prayer"}}).apply(newPlayer(t), nil); err == nil {
		t.Error("prayer mode without a location should be rejected")
	}
}

func TestRotationRequest(t *testing.T) {
	r, err := defaultRotation(41.88, -87.63, "ISNA", false).scene()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Items) != 3 || r.req.Items[2].Moon.South {
		t.Errorf("default rotation: %+v", r.req)
	}
	for name, req := range map[string]rotationRequest{
		"empty":              {},
		"too short":          {Items: []rotationItem{{sceneSpec{Mode: "moon", Moon: &moonRequest{}}, 0.5}}},
		"bad view":           {Items: []rotationItem{{sceneSpec{Mode: "planet", Planet: &planetRequest{Body: "vulcan"}}, 10}}},
		"connect4":           {Items: []rotationItem{{sceneSpec{Mode: "connect4"}, 10}}},
		"no prayer location": {Items: []rotationItem{{sceneSpec{Mode: "prayer", Prayer: &prayerRequest{}}, 10}}},
	} {
		if _, err := req.scene(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestLabelsDefaultOn(t *testing.T) {
	var req rotationRequest
	if err := json.Unmarshal([]byte(`{"items":[{"mode":"planet","planet":{"body":"mars"}},{"mode":"pokemon","pokemon":{"pokemon":"1","label":false}}]}`), &req); err != nil {
		t.Fatal(err)
	}
	if !req.Items[0].Planet.Label || req.Items[1].Pokemon.Label {
		t.Errorf("planet label %v (want true), pokemon label %v (want false)", req.Items[0].Planet.Label, req.Items[1].Pokemon.Label)
	}
}

func TestCitizenStateRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		png.Encode(w, image.NewNRGBA(image.Rect(0, 0, 64, 64)))
	}))
	defer srv.Close()
	saved := citizenSource
	citizenSource = &neotokyo.CitizenSource{BaseURL: srv.URL}
	defer func() { citizenSource = saved }()

	for _, req := range []citizenRequest{
		{Season: 2, ID: 2648, Zoom: false, Label: true, Dwell: 20},
		{Season: 0, ID: 0, Zoom: true, Label: false, Dwell: 15}, // random from either season
	} {
		f := &stateFile{path: filepath.Join(t.TempDir(), "state.json")}
		before := newPlayer(t)
		s, err := req.scene()
		if err != nil {
			t.Fatal(err)
		}
		before.Show(s)
		want := snapshot(before)
		if want.Citizen == nil || *want.Citizen != req {
			t.Errorf("snapshot %+v, want %+v", want.Citizen, req)
		}
		if err := f.save(want); err != nil {
			t.Fatal(err)
		}
		loaded, err := f.load()
		if err != nil {
			t.Fatal(err)
		}
		after := newPlayer(t)
		if err := loaded.apply(after, func(*connect4.Game) {}); err != nil {
			t.Fatal(err)
		}
		if got := snapshot(after); !reflect.DeepEqual(got, want) {
			t.Errorf("restored %+v, want %+v", got.Citizen, want.Citizen)
		}
	}
}

func TestCitizenRequestDefaults(t *testing.T) {
	var r citizenRequest
	if err := json.Unmarshal([]byte(`{"id":5}`), &r); err != nil || !r.Zoom || !r.Label {
		t.Errorf("%+v %v", r, err)
	}
	if err := json.Unmarshal([]byte(`{"zoom":false,"label":false}`), &r); err != nil || r.Zoom || r.Label {
		t.Errorf("%+v %v", r, err)
	}
}

func TestSavedScenes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := &stateFile{path: path}
	player := newPlayer(t)
	persist := func() {
		if err := state.save(snapshot(player)); err != nil {
			t.Fatal(err)
		}
	}
	mux := http.NewServeMux()
	handleScenes(mux, player, state, func(*connect4.Game) {}, persist)
	do := func(method, target, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, target, strings.NewReader(body)))
		return w
	}

	player.SetBrightness(40)
	player.ShowText(matrix.Text{Text: "WELCOME", Color: color.RGBA{0, 255, 0, 255}})
	if w := do("POST", "/api/scenes", `{"name":" Welcome "}`); w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	player.Show(moonphase.New(false))
	if w := do("POST", "/api/scenes", `{"name":"Moon"}`); w.Code != http.StatusOK {
		t.Fatalf("save moon: %d %s", w.Code, w.Body)
	}
	for _, body := range []string{`{"name":""}`, `{"name":"a/b"}`, `{"name":"` + strings.Repeat("x", 41) + `"}`, `nope`} {
		if w := do("POST", "/api/scenes", body); w.Code != http.StatusBadRequest {
			t.Errorf("save %s: %d, want 400", body, w.Code)
		}
	}

	// Saving the current display keeps the scenes, and they survive a restart.
	persist()
	reloaded := &stateFile{path: path}
	if _, err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	if got := reloaded.sceneList(); len(got) != 2 || got[0].Name != "Welcome" || got[0].Text.Text != "WELCOME" || got[1].Mode != "moon" {
		t.Fatalf("reloaded scenes: %+v", got)
	}

	player.SetBrightness(90)
	w := do("POST", "/api/scenes/Welcome/show", "")
	if w.Code != http.StatusOK {
		t.Fatalf("show: %d %s", w.Code, w.Body)
	}
	if s := player.State(); s.Brightness != 90 {
		t.Errorf("brightness after show = %d, want 90 (unchanged)", s.Brightness)
	}
	if spec, _ := specOf(player.State().Scene); spec.Text == nil || spec.Text.Text != "WELCOME" || spec.Text.Color != "#00ff00" {
		t.Errorf("shown scene: %+v", spec.Text)
	}
	if w := do("POST", "/api/scenes/Nope/show", ""); w.Code != http.StatusNotFound {
		t.Errorf("show missing: %d", w.Code)
	}

	// Saving under an existing name replaces that scene in place.
	if w := do("POST", "/api/scenes", `{"name":"Moon"}`); w.Code != http.StatusOK {
		t.Fatal(w.Body)
	}
	if got := state.sceneList(); len(got) != 2 || got[1].Name != "Moon" || got[1].Mode != "text" {
		t.Errorf("after replacing: %+v", got)
	}

	if w := do("DELETE", "/api/scenes/Moon", ""); w.Code != http.StatusOK {
		t.Fatalf("delete: %d", w.Code)
	}
	if w := do("DELETE", "/api/scenes/Moon", ""); w.Code != http.StatusNotFound {
		t.Errorf("delete again: %d", w.Code)
	}
	var listed []savedScene
	json.Unmarshal(do("GET", "/api/scenes", "").Body.Bytes(), &listed)
	if len(listed) != 1 || listed[0].Name != "Welcome" {
		t.Errorf("listed: %+v", listed)
	}

	player.Show(connect4.New(connect4.Options{}))
	if w := do("POST", "/api/scenes", `{"name":"Game"}`); w.Code != http.StatusConflict {
		t.Errorf("saving a game: %d, want 409", w.Code)
	}
}
