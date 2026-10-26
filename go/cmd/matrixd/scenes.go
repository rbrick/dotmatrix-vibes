package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"dotmatrix/matrix"
	"dotmatrix/minecraft"
	"dotmatrix/moonphase"
	"dotmatrix/neotokyo"
	"dotmatrix/orrery"
	"dotmatrix/planets"
	"dotmatrix/pokemon"
	"dotmatrix/prayerclock"
	"dotmatrix/starfront"
	"dotmatrix/weather"
)

// sceneSpec describes a scene in JSON, so it can be rebuilt: what the panel
// shows, or one view of a rotation. Mode says which of the other fields is
// set.
type sceneSpec struct {
	Mode    string          `json:"mode"` // "text", "prayer", "planet", "pokemon", "moon", "solarsystem", "minecraft", "starfront", "bytes" or "citizen"
	Text    *textRequest    `json:"text,omitempty"`
	Prayer  *prayerRequest  `json:"prayer,omitempty"`
	Planet  *planetRequest  `json:"planet,omitempty"`
	Pokemon *pokemonRequest `json:"pokemon,omitempty"`
	Moon    *moonRequest    `json:"moon,omitempty"`
	Solar   *solarRequest   `json:"solarsystem,omitempty"`
	MC      *mcRequest      `json:"minecraft,omitempty"`
	Roof    *roofRequest    `json:"starfront,omitempty"`
	Bytes   *bytesRequest   `json:"bytes,omitempty"`
	Citizen *citizenRequest `json:"citizen,omitempty"`
	Weather *weatherRequest `json:"weather,omitempty"`
}

type weatherRequest struct {
	Lat            float64 `json:"lat"`
	Long           float64 `json:"long"`
	Refresh        float64 `json:"refresh,omitempty"`
	Preview        string  `json:"preview,omitempty"`
	WeatherPreview string  `json:"weatherPreview,omitempty"`
}

func (r weatherRequest) scene() (*weather.Scene, error) {
	return weather.New(weather.Options{Lat: r.Lat, Long: r.Long, Interval: time.Duration(r.Refresh * float64(time.Second)), Preview: r.Preview, WeatherPreview: r.WeatherPreview})
}

type bytesRequest struct {
	Refresh float64 `json:"refresh,omitempty"` // seconds between checks; default 30, minimum 10
}

func (r bytesRequest) scene() (*neotokyo.PriceScene, error) {
	return neotokyo.NewPrice(neotokyo.PriceOptions{Interval: time.Duration(r.Refresh * float64(time.Second))})
}

type citizenRequest struct {
	Season int     `json:"season,omitempty"` // 1 or 2; 0 with id 0 is either
	ID     int     `json:"id,omitempty"`     // 0 is a different random Citizen every dwell
	Zoom   bool    `json:"zoom"`             // the face at full detail
	Label  bool    `json:"label"`
	Dwell  float64 `json:"dwell,omitempty"` // seconds per Citizen when random
}

// UnmarshalJSON zooms in and shows the label unless told not to.
func (r *citizenRequest) UnmarshalJSON(b []byte) error {
	type plain citizenRequest
	p := plain{Zoom: true, Label: true}
	err := json.Unmarshal(b, &p)
	*r = citizenRequest(p)
	return err
}

var citizenSource = neotokyo.DefaultCitizenSource()

func (r citizenRequest) scene() (*neotokyo.CitizenScene, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return neotokyo.NewCitizen(ctx, citizenSource, neotokyo.CitizenOptions{
		Season: r.Season, ID: r.ID, Zoom: r.Zoom, Label: r.Label,
		Dwell: time.Duration(r.Dwell * float64(time.Second)),
	})
}

type roofRequest struct {
	Building int     `json:"building,omitempty"` // a device number, or 0 for the consensus of all
	Refresh  float64 `json:"refresh,omitempty"`  // seconds between checks; default 15, minimum 5
}

func (r roofRequest) scene() (*starfront.Scene, error) {
	return starfront.New(starfront.Options{
		Building: r.Building, Interval: time.Duration(r.Refresh * float64(time.Second)),
	})
}

type mcRequest struct {
	Address string  `json:"address"`
	Icon    string  `json:"icon,omitempty"`    // "grass" (default) or "server"
	Refresh float64 `json:"refresh,omitempty"` // seconds between pings; default 5, minimum 2
}

func (r mcRequest) scene() (*minecraft.Scene, error) {
	return minecraft.New(minecraft.Options{
		Address: r.Address, Icon: r.Icon, Interval: time.Duration(r.Refresh * float64(time.Second)),
	})
}

type solarRequest struct {
	Speed float64 `json:"speed,omitempty"` // simulated days per second; 0 is now
	Label bool    `json:"label"`
}

// UnmarshalJSON names the planets unless "label" is false.
func (r *solarRequest) UnmarshalJSON(b []byte) error {
	type plain solarRequest
	p := plain{Label: true}
	err := json.Unmarshal(b, &p)
	*r = solarRequest(p)
	return err
}

type moonRequest struct {
	South bool `json:"south"` // as seen from the southern hemisphere
}

// build makes the scene a spec describes.
func (s sceneSpec) build() (matrix.Scene, error) {
	missing := func() error { return fmt.Errorf("mode %q without settings", s.Mode) }
	switch s.Mode {
	case "text":
		if s.Text == nil {
			return nil, missing()
		}
		t, err := textFromRequest(*s.Text)
		return t.WithDefaults(), err
	case "prayer":
		if s.Prayer == nil || s.Prayer.Lat == nil || s.Prayer.Long == nil {
			return nil, errors.New(`mode "prayer" without a location`)
		}
		return s.Prayer.scene()
	case "planet":
		if s.Planet == nil {
			return nil, missing()
		}
		return s.Planet.scene()
	case "pokemon":
		if s.Pokemon == nil {
			return nil, missing()
		}
		return s.Pokemon.scene()
	case "moon":
		if s.Moon == nil {
			return nil, missing()
		}
		return moonphase.New(s.Moon.South), nil
	case "minecraft":
		if s.MC == nil {
			return nil, missing()
		}
		return s.MC.scene()
	case "starfront":
		if s.Roof == nil {
			return nil, missing()
		}
		return s.Roof.scene()
	case "bytes":
		if s.Bytes == nil {
			return nil, missing()
		}
		return s.Bytes.scene()
	case "citizen":
		if s.Citizen == nil {
			return nil, missing()
		}
		return s.Citizen.scene()
	case "solarsystem":
		if s.Solar == nil {
			return nil, missing()
		}
		return orrery.New(orrery.Options{Speed: s.Solar.Speed, Label: s.Solar.Label}), nil
	case "weather":
		if s.Weather == nil {
			return nil, missing()
		}
		return s.Weather.scene()
	}
	return nil, fmt.Errorf("unknown mode %q", s.Mode)
}

// specOf describes a scene that build can make, or reports false.
func specOf(scene matrix.Scene) (sceneSpec, bool) {
	switch sc := scene.(type) {
	case matrix.Text:
		return sceneSpec{Mode: "text", Text: &textRequest{
			Text: sc.Text, Color: matrix.HexColor(sc.Color), Background: matrix.HexColor(sc.Background),
			Align: string(sc.Align), Scroll: string(sc.Scroll), Speed: sc.Speed,
		}}, true
	case *prayerclock.Clock:
		return sceneSpec{Mode: "prayer", Prayer: &prayerRequest{Lat: &sc.Lat, Long: &sc.Long, Method: sc.MethodName, Clock24: sc.Clock24}}, true
	case *planets.Scene:
		return sceneSpec{Mode: "planet", Planet: &planetRequest{Body: sc.Body, Label: sc.Label, Speed: sc.Speed, Dwell: sc.Dwell.Seconds()}}, true
	case *pokemon.Scene:
		return sceneSpec{Mode: "pokemon", Pokemon: &pokemonRequest{Pokemon: sc.Pokemon, Shiny: sc.Shiny, Label: sc.Label, Dwell: sc.Dwell.Seconds()}}, true
	case *moonphase.Scene:
		return sceneSpec{Mode: "moon", Moon: &moonRequest{South: sc.South}}, true
	case *minecraft.Scene:
		return sceneSpec{Mode: "minecraft", MC: &mcRequest{Address: sc.Address, Icon: sc.Icon, Refresh: sc.Interval.Seconds()}}, true
	case *starfront.Scene:
		return sceneSpec{Mode: "starfront", Roof: &roofRequest{Building: sc.Building, Refresh: sc.Interval.Seconds()}}, true
	case *neotokyo.PriceScene:
		return sceneSpec{Mode: "bytes", Bytes: &bytesRequest{Refresh: sc.Interval.Seconds()}}, true
	case *neotokyo.CitizenScene:
		return sceneSpec{Mode: "citizen", Citizen: &citizenRequest{
			Season: sc.Season, ID: sc.ID, Zoom: sc.Zoom, Label: sc.Label, Dwell: sc.Dwell.Seconds(),
		}}, true
	case *orrery.Scene:
		return sceneSpec{Mode: "solarsystem", Solar: &solarRequest{Speed: sc.Speed, Label: sc.Label}}, true
	case *weather.Scene:
		return sceneSpec{Mode: "weather", Weather: &weatherRequest{Lat: sc.Lat, Long: sc.Long, Refresh: sc.Interval.Seconds(), Preview: sc.Preview, WeatherPreview: sc.WeatherPreview}}, true
	}
	return sceneSpec{}, false
}

// rotationRequest is a list of views that take turns on the panel.
type rotationRequest struct {
	Items []rotationItem `json:"items"`
}

type rotationItem struct {
	sceneSpec
	Seconds float64 `json:"seconds"` // how long the view stays up each turn
}

// rotation is a matrix.Rotation that remembers how it was described, so it
// can be saved.
type rotation struct {
	*matrix.Rotation
	req rotationRequest
}

const maxRotationItems = 20

func (r rotationRequest) scene() (*rotation, error) {
	if len(r.Items) > maxRotationItems {
		return nil, fmt.Errorf("rotation: at most %d views", maxRotationItems)
	}
	saved := rotationRequest{Items: append([]rotationItem(nil), r.Items...)}
	items := make([]matrix.RotationItem, len(r.Items))
	for i, it := range r.Items {
		if it.Mode == "connect4" || it.Mode == "image" || it.Mode == "rotation" {
			return nil, fmt.Errorf("rotation view %d: %s can't be part of a rotation", i+1, it.Mode)
		}
		s, err := it.build()
		if err != nil {
			return nil, fmt.Errorf("rotation view %d: %w", i+1, err)
		}
		if it.Seconds < 1 {
			return nil, fmt.Errorf("rotation view %d: seconds must be at least 1", i+1)
		}
		items[i] = matrix.RotationItem{Scene: s, Duration: time.Duration(it.Seconds * float64(time.Second))}
		// Save what was resolved, e.g. a Pokémon's name instead of its number.
		if spec, ok := specOf(s); ok {
			saved.Items[i].sceneSpec = spec
		}
	}
	rot, err := matrix.NewRotation(items...)
	if err != nil {
		return nil, err
	}
	return &rotation{rot, saved}, nil
}

// defaultRotation is what -rotate shows: prayer times for three minutes,
// then the next planet of the tour, then the Moon's phase.
func defaultRotation(lat, long float64, method string, clock24 bool) rotationRequest {
	return rotationRequest{Items: []rotationItem{
		{sceneSpec{Mode: "prayer", Prayer: &prayerRequest{Lat: &lat, Long: &long, Method: method, Clock24: clock24}}, 180},
		// One planet per turn: the tour moves on by one each time round.
		{sceneSpec{Mode: "planet", Planet: &planetRequest{Body: planets.Tour, Label: true, Dwell: 20}}, 20},
		{sceneSpec{Mode: "moon", Moon: &moonRequest{South: lat < 0}}, 20},
	}}
}
