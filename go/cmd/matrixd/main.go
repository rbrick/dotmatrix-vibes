// Command matrixd serves an HTTP API (and a small control page) for the LED
// panel.
//
//	POST /api/text        {"text":"Hello","color":"#ff0000","background":"#000",
//	                       "align":"center","scroll":"auto","speed":24}
//	                      or a text/plain body containing just the text
//	POST /api/image       PNG, JPEG or GIF body; scaled down to fit
//	POST /api/brightness  {"brightness":0-255}
//	POST /api/clear
//	GET  /api/state
//	GET  /api/emoji       built-in emoji: [{"name":"heart","char":"❤"}, ...]
//	POST /api/prayer      {"lat":40.71,"long":-74.01,"method":"ISNA","clock24":false}
//	                      shows the next prayer time and a time-of-day emoji
//	POST /api/connect4/new     {"vsComputer":true,"difficulty":"easy|normal|hard","cpuFirst":false}
//	POST /api/connect4/move    {"column":0-6}, or {} to drop at the cursor
//	POST /api/connect4/cursor  {"column":0-6} or {"delta":-1|1}
//	GET  /api/connect4         board, turn, winner and score
//	POST /api/planet      {"body":"saturn"|"tour","label":true,"speed":1,"dwell":12}
//	                      a rotating planet; "tour" cycles through the solar system
//	POST /api/pokemon     {"pokemon":"pikachu"|"25"|"random","shiny":false,"label":true,"dwell":10}
//	                      a Pokémon box sprite from PokéSprite (downloaded once, then cached)
//	GET  /api/pokemon/list  every Pokémon: [{"dex":1,"name":"Bulbasaur"}, ...]
//	GET  /api/pokemon/balls every Poké Ball: [{"name":"Poké Ball","shortcode":"pokeball"}, ...]
//	GET  /api/pokemon/balls/{shortcode}.png  a ball's sprite
//	POST /api/weather     {"lat":40.71,"long":-74.01,"refresh":600,
//	                       "preview":"sunset","weatherPreview":"storm"}
//	                      a full-panel Open-Meteo landscape with optional previews
//	POST /api/moon        {"south":false}
//	                      today's phase of the Moon, and the date of the next full or new moon
//	POST /api/solarsystem {"speed":0,"label":true}
//	                      every planet where it really is around the Sun (directions
//	                      exact, distances compressed); speed > 0 is a time-lapse in
//	                      days per second
//	POST /api/minecraft   {"address":"mc.hypixel.net","icon":"grass"|"server","refresh":5}
//	                      pings a Minecraft Java server every refresh seconds (default 5,
//	                      minimum 2) and shows its address,
//	                      a grass block (or the server's icon), latency and player count;
//	                      the response includes the first ping's result
//	POST /api/starfront   {"building":0,"refresh":15}
//	                      whether the Starfront Observatory's roll-off roofs are open,
//	                      checked every refresh seconds (default 15, minimum 5); the
//	                      headline is what most buildings report, or building's own
//	                      state when one is given; the response includes the first check
//	GET  /api/starfront/roofs  every building: [{"device_number":1,"name":"Building 1",
//	                      "is_open":false,"state_update":"..."}, ...]
//	POST /api/bytes       {"refresh":30}
//	                      the price of Neo Tokyo's $BYTES token (api.bytes.onl), checked
//	                      every refresh seconds (default 30, minimum 10)
//	POST /api/citizen     {"season":1,"id":2648,"zoom":true,"label":true,"dwell":20}
//	                      a Neo Tokyo Citizen from citgen.rcw.io (downloaded once, then
//	                      cached); id 0 shows a random one every dwell seconds, from
//	                      season 1 or 2, or either when season is 0
//	POST /api/rotation    {"items":[{"mode":"prayer","prayer":{...},"seconds":180},
//	                                {"mode":"planet","planet":{...},"seconds":20},
//	                                {"mode":"moon","moon":{},"seconds":20}]}
//	                      takes turns showing views; each item is a mode ("text",
//	                      "prayer", "weather", "planet", "pokemon", "moon", "solarsystem", "minecraft", "starfront", "bytes" or "citizen") with
//	                      the same
//	                      settings as that mode's endpoint
//	GET    /api/scenes             saved scenes: [{"name":"Welcome","mode":"text",...}, ...]
//	POST   /api/scenes             {"name":"Welcome"} saves what the panel shows now under
//	                               that name, replacing any scene with the same name
//	POST   /api/scenes/{name}/show shows a saved scene; brightness stays as it is
//	DELETE /api/scenes/{name}
//
// Text may contain emoji characters or :shortcodes: such as :heart:,
// Pokémon box sprites as :pikachu: or :shiny-pikachu:, and Poké Balls as
// :pokeball:, :greatball:, :masterball: and so on.
//
// What the panel shows and its brightness are saved to a state file (by
// default ~/.config/matrixd/state.json) after every change and restored at
// startup, unless -text, -prayer or -rotate says what to show. The saved
// scenes are kept in the same file.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"dotmatrix/connect4"
	"dotmatrix/gamepad"
	"dotmatrix/matrix"
	"dotmatrix/menu"
	"dotmatrix/minecraft"
	"dotmatrix/moonphase"
	"dotmatrix/neotokyo"
	"dotmatrix/orrery"
	"dotmatrix/planets"
	"dotmatrix/pokemon"
	"dotmatrix/prayerclock"
	"dotmatrix/starfront"
	"dotmatrix/tetris"
)

//go:embed index.html
var static embed.FS

type textRequest struct {
	Text       string  `json:"text"`
	Color      string  `json:"color,omitempty"`
	Background string  `json:"background,omitempty"`
	Align      string  `json:"align,omitempty"`
	Scroll     string  `json:"scroll,omitempty"`
	Speed      float64 `json:"speed,omitempty"`
}

type prayerRequest struct {
	Lat     *float64 `json:"lat"`
	Long    *float64 `json:"long"`
	Method  string   `json:"method"`
	Clock24 bool     `json:"clock24"`
}

func (r prayerRequest) scene() (*prayerclock.Clock, error) {
	if r.Method == "" {
		r.Method = "ISNA"
	}
	return prayerclock.New(*r.Lat, *r.Long, r.Method, r.Clock24)
}

type planetRequest struct {
	Body  string  `json:"body"` // a planet name, or "tour"
	Label bool    `json:"label"`
	Speed float64 `json:"speed,omitempty"`
	Dwell float64 `json:"dwell,omitempty"` // seconds per body on the tour
}

// UnmarshalJSON shows the name unless "label" is false.
func (r *planetRequest) UnmarshalJSON(b []byte) error {
	type plain planetRequest
	p := plain{Label: true}
	err := json.Unmarshal(b, &p)
	*r = planetRequest(p)
	return err
}

func (r planetRequest) scene() (*planets.Scene, error) {
	return planets.New(planets.Options{
		Body: r.Body, Label: r.Label, Speed: r.Speed,
		Dwell: time.Duration(r.Dwell * float64(time.Second)),
	})
}

type pokemonRequest struct {
	Pokemon string  `json:"pokemon"` // name, Pokédex number or "random"
	Shiny   bool    `json:"shiny"`
	Label   bool    `json:"label"`
	Dwell   float64 `json:"dwell,omitempty"` // seconds per Pokémon when random
}

// UnmarshalJSON shows the name unless "label" is false.
func (r *pokemonRequest) UnmarshalJSON(b []byte) error {
	type plain pokemonRequest
	p := plain{Label: true}
	err := json.Unmarshal(b, &p)
	*r = pokemonRequest(p)
	return err
}

var spriteSource = pokemon.DefaultSource()

func (r pokemonRequest) scene() (*pokemon.Scene, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return pokemon.New(ctx, spriteSource, pokemon.Options{
		Pokemon: r.Pokemon, Shiny: r.Shiny, Label: r.Label,
		Dwell: time.Duration(r.Dwell * float64(time.Second)),
	})
}

type stateResponse struct {
	Connected bool   `json:"connected"`
	Port      string `json:"port,omitempty"`
	Error     string `json:"error,omitempty"`
	content
	MCStatus   *mcStatus   `json:"minecraftStatus,omitempty"` // when showing a Minecraft server
	RoofStatus *roofStatus `json:"starfrontStatus,omitempty"` // when showing Starfront
	BytesPrice *bytesPrice `json:"bytesPrice,omitempty"`      // when showing $BYTES
	Citizen    *citizenNow `json:"citizenShown,omitempty"`    // when showing a Citizen
}

// bytesPrice is the latest $BYTES price.
type bytesPrice struct {
	USD       float64 `json:"usd"`
	ETH       float64 `json:"eth"`
	MarketCap float64 `json:"marketCap"`
	Error     string  `json:"error,omitempty"`
}

func bytesPriceOf(s *neotokyo.PriceScene) *bytesPrice {
	p, err := s.Price()
	out := &bytesPrice{}
	switch {
	case p == nil && err == nil:
		return nil // not checked yet
	case p != nil:
		out.USD, out.ETH, out.MarketCap = p.USD, p.ETH, p.MarketCap
	}
	if err != nil {
		out.Error = err.Error()
	}
	return out
}

// citizenNow is the Citizen on the panel.
type citizenNow struct {
	Season int `json:"season"`
	ID     int `json:"id"`
}

// roofStatus is the latest reading of Starfront's roofs.
type roofStatus struct {
	Roofs []starfront.Roof `json:"roofs"`
	Error string           `json:"error,omitempty"`
}

func roofStatusOf(s *starfront.Scene) *roofStatus {
	roofs, err := s.Roofs()
	if roofs == nil && err == nil {
		return nil // not checked yet
	}
	st := &roofStatus{Roofs: roofs}
	if err != nil {
		st.Error = err.Error()
	}
	return st
}

// mcStatus is the latest ping of the Minecraft server on the panel.
type mcStatus struct {
	Online    bool    `json:"online"`
	Error     string  `json:"error,omitempty"`
	Players   int     `json:"players"`
	Max       int     `json:"max"`
	Version   string  `json:"version,omitempty"`
	MOTD      string  `json:"motd,omitempty"`
	LatencyMS float64 `json:"latencyMs,omitempty"`
}

func statusOf(s *minecraft.Scene) *mcStatus {
	st, err := s.Status()
	switch {
	case err != nil:
		return &mcStatus{Error: err.Error()}
	case st == nil:
		return nil // not pinged yet
	}
	return &mcStatus{
		Online: true, Players: st.Players.Online, Max: st.Players.Max, Version: st.VersionName(),
		MOTD: st.MOTD(), LatencyMS: float64(st.Latency.Microseconds()) / 1000,
	}
}

const maxBody = 4 << 20

func showControllerDisplay(player *matrix.Player, spec sceneSpec, persist func()) {
	scene, err := spec.build()
	if err != nil {
		log.Printf("controller menu: %v", err)
		return
	}
	player.Show(scene)
	persist()
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP listen address (use :8080 to allow other machines)")
	port := flag.String("port", "", "serial port of the Pico (default: find automatically)")
	text := flag.String("text", "HELLO!", "text to show at startup")
	showPrayer := flag.Bool("prayer", false, "show prayer times at startup (needs -lat and -long)")
	lat := flag.Float64("lat", 0, "latitude for prayer times")
	long := flag.Float64("long", 0, "longitude for prayer times")
	method := flag.String("method", "ISNA", "prayer calculation method: "+strings.Join(prayerclock.MethodNames(), ", "))
	clock24 := flag.Bool("clock24", false, "24-hour times on the prayer clock")
	rotate := flag.Bool("rotate", false, "rotate through prayer times, planets and the moon phase at startup (needs -lat and -long)")
	statePath := flag.String("state", defaultStatePath(), `file to save and restore the display state ("" to disable)`)
	useGamepad := flag.Bool("gamepad", true, "listen for Linux game controllers")
	flag.Parse()
	explicit := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })

	player := matrix.NewPlayer(*port)
	defer player.Close()
	state := &stateFile{path: *statePath}
	persist := func() {
		if err := state.save(snapshot(player)); err != nil {
			log.Printf("saving state: %v", err)
		}
	}
	// The computer moves in the background; show and save its moves too.
	attachGame := func(g *connect4.Game) {
		g.SetOnChange(func() {
			player.Redraw()
			persist()
		})
	}

	if (*showPrayer || *rotate) && (!explicit["lat"] || !explicit["long"]) {
		log.Fatal("-prayer and -rotate need -lat and -long")
	}
	switch saved, err := state.load(); {
	case *rotate:
		r, err := defaultRotation(*lat, *long, *method, *clock24).scene()
		if err != nil {
			log.Fatal(err)
		}
		player.Show(r)
		persist()
	case *showPrayer:
		clock, err := prayerclock.New(*lat, *long, *method, *clock24)
		if err != nil {
			log.Fatal(err)
		}
		player.Show(clock)
		persist()
	case explicit["text"]:
		player.ShowText(matrix.Text{Text: *text})
		persist()
	case err == nil:
		if err := saved.apply(player, attachGame); err != nil {
			log.Printf("ignoring saved state in %s: %v", state.path, err)
			player.ShowText(matrix.Text{Text: *text})
		} else {
			log.Printf("restored %s display from %s", saved.Mode, state.path)
		}
	default:
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("ignoring saved state: %v", err)
		}
		player.ShowText(matrix.Text{Text: *text})
	}

	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(static))
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		writeState(w, player)
	})
	mux.HandleFunc("GET /api/emoji", func(w http.ResponseWriter, r *http.Request) {
		type item struct {
			Name string `json:"name"`
			Char string `json:"char"`
		}
		var list []item
		for _, e := range matrix.Emojis() {
			list = append(list, item{e.Name, e.Char})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
	})
	mux.HandleFunc("POST /api/text", func(w http.ResponseWriter, r *http.Request) {
		t, err := decodeText(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		player.ShowText(t)
		persist()
		writeState(w, player)
	})
	mux.HandleFunc("POST /api/prayer", func(w http.ResponseWriter, r *http.Request) {
		var req prayerRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil ||
			req.Lat == nil || req.Long == nil {
			http.Error(w, `expected {"lat": ..., "long": ..., "method": "ISNA", "clock24": false}`, http.StatusBadRequest)
			return
		}
		clock, err := req.scene()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		player.Show(clock)
		persist()
		writeState(w, player)
	})
	mux.HandleFunc("POST /api/image", func(w http.ResponseWriter, r *http.Request) {
		img, _, err := image.Decode(http.MaxBytesReader(w, r.Body, maxBody))
		if err != nil {
			http.Error(w, "decoding image: "+err.Error(), http.StatusBadRequest)
			return
		}
		player.ShowImage(img)
		persist()
		writeState(w, player)
	})
	mux.HandleFunc("POST /api/brightness", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Brightness *int `json:"brightness"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil ||
			req.Brightness == nil || *req.Brightness < 0 || *req.Brightness > 255 {
			http.Error(w, `expected {"brightness": 0-255}`, http.StatusBadRequest)
			return
		}
		player.SetBrightness(uint8(*req.Brightness))
		persist()
		writeState(w, player)
	})
	mux.HandleFunc("POST /api/clear", func(w http.ResponseWriter, r *http.Request) {
		player.Clear()
		persist()
		writeState(w, player)
	})

	handleConnect4(mux, player, attachGame, persist)
	handleBluetooth(mux)
	if *useGamepad {
		var controllerMu sync.Mutex
		var previous matrix.Scene
		var lastMenu *menu.Menu
		menuPositions := make(map[string]int)
		showMenu := func(title string, items []menu.Item) {
			if current, ok := player.State().Scene.(*menu.Menu); ok {
				menuPositions[current.Title] = current.Index()
			}
			player.Show(menu.NewAt(title, items, menuPositions[title]))
		}
		openGlobal := func() {
			current := player.State().Scene
			if _, inMenu := current.(*menu.Menu); !inMenu {
				previous = current
			}
			showMenu("MENU", []menu.Item{
				{ID: "games", Label: "GAMES"},
				{ID: "displays", Label: "DISPLAYS"},
				{ID: "resume", Label: "RESUME"},
				{ID: "off", Label: "DISPLAY OFF"},
			})
		}
		openGames := func() {
			showMenu("GAMES", []menu.Item{
				{ID: "connect4", Label: "CONNECT 4"},
				{ID: "tetris", Label: "BLOCK DROP"},
				{ID: "back:global", Label: "BACK"},
			})
		}
		var configuredLat, configuredLong float64
		var hasConfiguredLocation bool
		configuredLocation := func() (float64, float64, bool) {
			if hasConfiguredLocation {
				return configuredLat, configuredLong, true
			}
			locationOf := func(spec sceneSpec) (float64, float64, bool) {
				if spec.Weather != nil && (spec.Weather.Lat != 0 || spec.Weather.Long != 0) {
					return spec.Weather.Lat, spec.Weather.Long, true
				}
				if spec.Prayer != nil && spec.Prayer.Lat != nil && spec.Prayer.Long != nil {
					return *spec.Prayer.Lat, *spec.Prayer.Long, true
				}
				return 0, 0, false
			}
			if previous != nil {
				if spec, ok := specOf(previous); ok {
					if lat, long, ok := locationOf(spec); ok {
						configuredLat, configuredLong, hasConfiguredLocation = lat, long, true
						return lat, long, true
					}
				}
			}
			for _, saved := range state.sceneList() {
				if lat, long, ok := locationOf(saved.sceneSpec); ok {
					configuredLat, configuredLong, hasConfiguredLocation = lat, long, true
					return lat, long, true
				}
			}
			return 0, 0, false
		}
		openDisplays := func() {
			items := []menu.Item{
				{ID: "config:planet", Label: "PLANETS"},
				{ID: "config:solar", Label: "SOLAR SYSTEM"},
				{ID: "config:moon", Label: "MOON PHASE"},
				{ID: "config:pokemon", Label: "POKEMON"},
				{ID: "config:starfront", Label: "STARFRONT"},
				{ID: "config:bytes", Label: "BYTES PRICE"},
				{ID: "config:citizen", Label: "CITIZENS"},
			}
			if _, _, ok := configuredLocation(); ok {
				items = append(items, menu.Item{ID: "config:weather", Label: "WEATHER"}, menu.Item{ID: "config:prayer", Label: "PRAYER CLOCK"})
			}
			for i, scene := range state.sceneList() {
				items = append(items, menu.Item{ID: fmt.Sprintf("scene:%d", i), Label: scene.Name})
			}
			items = append(items, menu.Item{ID: "welcome", Label: "WELCOME"}, menu.Item{ID: "back:global", Label: "BACK"})
			showMenu("DISPLAYS", items)
		}
		openDisplayConfig := func(kind string) {
			back := menu.Item{ID: "back:displays", Label: "BACK"}
			var items []menu.Item
			switch kind {
			case "planet":
				for _, body := range []string{"tour", "sun", "mercury", "venus", "earth", "moon", "mars", "jupiter", "saturn", "uranus", "neptune", "pluto"} {
					items = append(items, menu.Item{ID: "display:planet:" + body, Label: strings.ToUpper(body)})
				}
			case "solar":
				items = []menu.Item{{ID: "display:solar:0", Label: "LIVE"}, {ID: "display:solar:1", Label: "1 DAY / SEC"}, {ID: "display:solar:30", Label: "30 DAYS / SEC"}}
			case "moon":
				items = []menu.Item{{ID: "display:moon:north", Label: "NORTHERN"}, {ID: "display:moon:south", Label: "SOUTHERN"}}
			case "pokemon":
				items = []menu.Item{{ID: "display:pokemon:normal", Label: "RANDOM"}, {ID: "display:pokemon:shiny", Label: "RANDOM SHINY"}}
			case "starfront":
				items = append(items, menu.Item{ID: "display:starfront:0", Label: "ALL BUILDINGS"})
				for i := 1; i <= 14; i++ {
					items = append(items, menu.Item{ID: fmt.Sprintf("display:starfront:%d", i), Label: fmt.Sprintf("BUILDING %d", i)})
				}
			case "bytes":
				items = []menu.Item{{ID: "display:bytes:30", Label: "UPDATE 30 SEC"}, {ID: "display:bytes:60", Label: "UPDATE 1 MIN"}, {ID: "display:bytes:300", Label: "UPDATE 5 MIN"}}
			case "citizen":
				items = []menu.Item{{ID: "display:citizen:0", Label: "EITHER SEASON"}, {ID: "display:citizen:1", Label: "SEASON 1"}, {ID: "display:citizen:2", Label: "SEASON 2"}}
			case "weather":
				items = []menu.Item{{ID: "display:weather:live", Label: "LIVE"}, {ID: "display:weather:sunrise", Label: "SUNRISE PREVIEW"}, {ID: "display:weather:sunset", Label: "SUNSET PREVIEW"}, {ID: "display:weather:night", Label: "NIGHT PREVIEW"}, {ID: "display:weather:rain", Label: "RAIN PREVIEW"}, {ID: "display:weather:storm", Label: "STORM PREVIEW"}}
			case "prayer":
				for _, method := range []string{"ISNA", "MWL", "EGAS", "Karachi", "UAE", "Kuwait"} {
					items = append(items, menu.Item{ID: "display:prayer:" + method, Label: strings.ToUpper(method)})
				}
			}
			items = append(items, back)
			showMenu(strings.ToUpper(kind), items)
		}
		go gamepad.Listen(context.Background(), func(action gamepad.Action) {
			controllerMu.Lock()
			defer controllerMu.Unlock()
			if action == gamepad.Home || action == gamepad.Start {
				if _, alreadyInMenu := player.State().Scene.(*menu.Menu); alreadyInMenu || lastMenu == nil {
					openGlobal()
				} else {
					player.Show(lastMenu)
				}
				return
			}
			if m, ok := player.State().Scene.(*menu.Menu); ok {
				switch action {
				case gamepad.Left, gamepad.Up:
					m.Move(-1)
					player.Redraw()
				case gamepad.Right, gamepad.Down:
					m.Move(1)
					player.Redraw()
				case gamepad.Select:
					item, exists := m.Current()
					if !exists {
						return
					}
					if kind, ok := strings.CutPrefix(item.ID, "config:"); ok {
						openDisplayConfig(kind)
						return
					}
					// Launching content remembers this exact menu object and cursor.
					// Home from the content can therefore return here, rather than
					// starting over at the root menu.
					if strings.HasPrefix(item.ID, "display:") || strings.HasPrefix(item.ID, "scene:") ||
						item.ID == "connect4" || item.ID == "tetris" || item.ID == "welcome" || item.ID == "off" {
						lastMenu = m
					}
					switch item.ID {
					case "games":
						openGames()
					case "displays":
						openDisplays()
					case "back:global":
						openGlobal()
					case "back:displays":
						openDisplays()
					case "resume":
						if previous != nil {
							player.Show(previous)
						}
					case "connect4":
						g := connect4.New(connect4.Options{VsComputer: true, Difficulty: connect4.Normal})
						attachGame(g)
						player.Show(g)
						persist()
					case "tetris":
						player.Show(tetris.New())
					case "welcome":
						player.ShowText(matrix.Text{Text: "WELCOME!"})
						persist()
					case "off":
						player.Clear()
						persist()
					default:
						parts := strings.Split(item.ID, ":")
						if len(parts) == 3 && parts[0] == "display" {
							value := parts[2]
							switch parts[1] {
							case "planet":
								showControllerDisplay(player, sceneSpec{Mode: "planet", Planet: &planetRequest{Body: value, Label: true, Dwell: 15}}, persist)
							case "solar":
								var speed float64
								fmt.Sscan(value, &speed)
								showControllerDisplay(player, sceneSpec{Mode: "solarsystem", Solar: &solarRequest{Speed: speed, Label: true}}, persist)
							case "moon":
								showControllerDisplay(player, sceneSpec{Mode: "moon", Moon: &moonRequest{South: value == "south"}}, persist)
							case "pokemon":
								showControllerDisplay(player, sceneSpec{Mode: "pokemon", Pokemon: &pokemonRequest{Pokemon: "random", Shiny: value == "shiny", Label: true, Dwell: 15}}, persist)
							case "starfront":
								var building int
								fmt.Sscan(value, &building)
								showControllerDisplay(player, sceneSpec{Mode: "starfront", Roof: &roofRequest{Building: building, Refresh: 15}}, persist)
							case "bytes":
								var refresh float64
								fmt.Sscan(value, &refresh)
								showControllerDisplay(player, sceneSpec{Mode: "bytes", Bytes: &bytesRequest{Refresh: refresh}}, persist)
							case "citizen":
								var season int
								fmt.Sscan(value, &season)
								showControllerDisplay(player, sceneSpec{Mode: "citizen", Citizen: &citizenRequest{Season: season, Zoom: true, Label: true, Dwell: 20}}, persist)
							case "weather":
								if lat, long, ok := configuredLocation(); ok {
									req := &weatherRequest{Lat: lat, Long: long, Refresh: 600}
									if value != "live" {
										if value == "rain" || value == "storm" {
											req.WeatherPreview = value
										} else {
											req.Preview = value
										}
									}
									showControllerDisplay(player, sceneSpec{Mode: "weather", Weather: req}, persist)
								}
							case "prayer":
								if lat, long, ok := configuredLocation(); ok {
									showControllerDisplay(player, sceneSpec{Mode: "prayer", Prayer: &prayerRequest{Lat: &lat, Long: &long, Method: value}}, persist)
								}
							}
							return
						}
						var index int
						if _, err := fmt.Sscanf(item.ID, "scene:%d", &index); err == nil {
							scenes := state.sceneList()
							if index >= 0 && index < len(scenes) {
								content := scenes[index].content
								content.Brightness = player.State().Brightness
								if err := content.apply(player, attachGame); err != nil {
									log.Printf("controller menu: %v", err)
								} else {
									persist()
								}
							}
						}
					}
				}
				return
			}
			switch g := player.State().Scene.(type) {
			case *connect4.Game:
				switch action {
				case gamepad.Left:
					g.MoveCursor(-1)
				case gamepad.Right:
					g.MoveCursor(1)
				case gamepad.Select, gamepad.Down:
					if err := g.DropAtCursor(); err != nil {
						log.Printf("gamepad move: %v", err)
					}
				default:
					return
				}
				player.Redraw()
				persist()
			case *tetris.Game:
				switch action {
				case gamepad.Left:
					g.Move(-1)
				case gamepad.Right:
					g.Move(1)
				case gamepad.Down:
					g.SoftDrop()
				case gamepad.Up, gamepad.Select:
					g.Rotate()
				default:
					return
				}
				player.Redraw()
			}
		})
	}
	handleScenes(mux, player, state, attachGame, persist)
	mux.HandleFunc("POST /api/pokemon", func(w http.ResponseWriter, r *http.Request) {
		var req pokemonRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		s, err := req.scene()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		player.Show(s)
		persist()
		writeState(w, player)
	})
	mux.HandleFunc("GET /api/pokemon/list", func(w http.ResponseWriter, r *http.Request) {
		type item struct {
			Dex  int    `json:"dex"`
			Name string `json:"name"`
		}
		list := make([]item, len(pokemon.Pokedex))
		for i, e := range pokemon.Pokedex {
			list[i] = item{e.Dex, e.Name}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
	})
	mux.HandleFunc("GET /api/pokemon/balls", func(w http.ResponseWriter, r *http.Request) {
		type item struct {
			Name      string `json:"name"`
			Shortcode string `json:"shortcode"`
		}
		list := make([]item, len(pokemon.Balls))
		for i, b := range pokemon.Balls {
			list[i] = item{b.Name(), b.Shortcode()}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
	})
	mux.HandleFunc("GET /api/pokemon/balls/{file}", func(w http.ResponseWriter, r *http.Request) {
		name, ok := strings.CutSuffix(r.PathValue("file"), ".png")
		b, err := pokemon.LookupBall(name)
		if !ok || err != nil {
			http.NotFound(w, r)
			return
		}
		data, err := spriteSource.BallPNG(r.Context(), b)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "max-age=86400")
		w.Write(data)
	})
	mux.HandleFunc("POST /api/planet", func(w http.ResponseWriter, r *http.Request) {
		var req planetRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		s, err := req.scene()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		player.Show(s)
		persist()
		writeState(w, player)
	})

	mux.HandleFunc("POST /api/weather", func(w http.ResponseWriter, r *http.Request) {
		var req weatherRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
			http.Error(w, `expected {"lat": ..., "long": ..., "refresh": 600}`, http.StatusBadRequest)
			return
		}
		s, err := req.scene()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		err = s.Refresh(ctx)
		cancel()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		player.Show(s)
		persist()
		writeState(w, player)
	})

	mux.HandleFunc("POST /api/moon", func(w http.ResponseWriter, r *http.Request) {
		var req moonRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil && err != io.EOF {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		player.Show(moonphase.New(req.South))
		persist()
		writeState(w, player)
	})
	mux.HandleFunc("POST /api/minecraft", func(w http.ResponseWriter, r *http.Request) {
		var req mcRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
			http.Error(w, `expected {"address": "host[:port]", "icon": "grass"}`, http.StatusBadRequest)
			return
		}
		s, err := req.scene()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Ping once before showing it, so the reply says whether it worked.
		// An unreachable server is still shown (as OFFLINE) and retried.
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		s.Refresh(ctx)
		cancel()
		player.Show(s)
		persist()
		writeState(w, player)
	})
	mux.HandleFunc("POST /api/starfront", func(w http.ResponseWriter, r *http.Request) {
		var req roofRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil && err != io.EOF {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		s, err := req.scene()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		s.Refresh(ctx)
		cancel()
		player.Show(s)
		persist()
		writeState(w, player)
	})
	mux.HandleFunc("GET /api/starfront/roofs", func(w http.ResponseWriter, r *http.Request) {
		roofs, err := starfront.Fetch(r.Context(), starfront.DefaultURL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(roofs)
	})
	mux.HandleFunc("POST /api/bytes", func(w http.ResponseWriter, r *http.Request) {
		var req bytesRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil && err != io.EOF {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		s, err := req.scene()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		s.Refresh(ctx)
		cancel()
		player.Show(s)
		persist()
		writeState(w, player)
	})
	mux.HandleFunc("POST /api/citizen", func(w http.ResponseWriter, r *http.Request) {
		req := citizenRequest{Zoom: true, Label: true}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil && err != io.EOF {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		s, err := req.scene()
		if err != nil {
			code := http.StatusBadGateway
			if errors.Is(err, neotokyo.ErrNoCitizen) || req.ID < 0 || req.Season < 0 || req.Season > 2 {
				code = http.StatusBadRequest
			}
			http.Error(w, err.Error(), code)
			return
		}
		player.Show(s)
		persist()
		writeState(w, player)
	})
	mux.HandleFunc("POST /api/solarsystem", func(w http.ResponseWriter, r *http.Request) {
		req := solarRequest{Label: true}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil && err != io.EOF {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Speed < 0 {
			http.Error(w, "speed can't be negative", http.StatusBadRequest)
			return
		}
		player.Show(orrery.New(orrery.Options{Speed: req.Speed, Label: req.Label}))
		persist()
		writeState(w, player)
	})
	mux.HandleFunc("POST /api/rotation", func(w http.ResponseWriter, r *http.Request) {
		var req rotationRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		rot, err := req.scene()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		player.Show(rot)
		persist()
		writeState(w, player)
	})

	log.Printf("matrixd listening on http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func decodeText(w http.ResponseWriter, r *http.Request) (matrix.Text, error) {
	body := http.MaxBytesReader(w, r.Body, maxBody)
	var req textRequest
	if ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); ct == "text/plain" {
		b, err := io.ReadAll(body)
		if err != nil {
			return matrix.Text{}, err
		}
		req.Text = string(b)
	} else if err := json.NewDecoder(body).Decode(&req); err != nil {
		return matrix.Text{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return textFromRequest(req)
}

func textFromRequest(req textRequest) (matrix.Text, error) {
	t := matrix.Text{Text: req.Text, Speed: req.Speed}
	var err error
	if req.Color != "" {
		if t.Color, err = matrix.ParseColor(req.Color); err != nil {
			return t, err
		}
	}
	if req.Background != "" {
		if t.Background, err = matrix.ParseColor(req.Background); err != nil {
			return t, err
		}
	}
	switch a := matrix.Align(req.Align); a {
	case "", matrix.AlignLeft, matrix.AlignCenter, matrix.AlignRight:
		t.Align = a
	default:
		return t, errors.New(`align must be "left", "center" or "right"`)
	}
	switch s := matrix.Scroll(req.Scroll); s {
	case "", matrix.ScrollAuto, matrix.ScrollAlways, matrix.ScrollNever:
		t.Scroll = s
	default:
		return t, errors.New(`scroll must be "auto", "always" or "never"`)
	}
	// Pokémon shortcodes like :pikachu: become inline sprites.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if t.Images, err = pokemon.InlineImages(ctx, spriteSource, t.Text); err != nil {
		return t, err
	}
	return t, nil
}

func writeState(w http.ResponseWriter, p *matrix.Player) {
	s := p.State()
	resp := stateResponse{Connected: s.Connected, Port: s.Port, Error: s.Error, content: snapshot(p)}
	resp.Image = nil // large and not useful to API clients
	if mc, ok := s.Scene.(*minecraft.Scene); ok {
		resp.MCStatus = statusOf(mc)
	}
	switch sc := s.Scene.(type) {
	case *starfront.Scene:
		resp.RoofStatus = roofStatusOf(sc)
	case *neotokyo.PriceScene:
		resp.BytesPrice = bytesPriceOf(sc)
	case *neotokyo.CitizenScene:
		season, id := sc.Current()
		resp.Citizen = &citizenNow{season, id}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
