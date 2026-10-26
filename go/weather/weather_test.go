package weather

import (
	"context"
	"image/color"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"dotmatrix/matrix"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFetchOnlyWeather(t *testing.T) {
	client := http.DefaultClient
	defer func() { http.DefaultClient = client }()
	http.DefaultClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Has("daily") || r.URL.Query().Get("current") != "weather_code,precipitation" {
			t.Errorf("weather request includes non-weather fields: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"current":{"weather_code":61,"precipitation":1.5}}`))}, nil
	})}
	got, err := Fetch(context.Background(), 41, -87)
	if err != nil || got.Code != 61 || got.Precipitation != 1.5 {
		t.Errorf("weather-only response: %+v, %v", got, err)
	}
}

func TestOptions(t *testing.T) {
	if _, err := New(Options{Lat: 91}); err == nil {
		t.Error("accepted invalid latitude")
	}
	if _, err := New(Options{Interval: time.Second}); err == nil {
		t.Error("accepted excessive refresh rate")
	}
	if _, err := New(Options{Preview: "yesterday"}); err == nil {
		t.Error("accepted unknown preview")
	}
	if _, err := New(Options{WeatherPreview: "tornado"}); err == nil {
		t.Error("accepted unknown weather preview")
	}
}

func TestWeatherPreview(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	s, _ := New(Options{})
	s.now = func() time.Time { return now }
	s.checked = now
	s.current = Conditions{}
	clear, rain, storm := matrix.NewCanvas(), matrix.NewCanvas(), matrix.NewCanvas()
	s.Render(clear, 0)
	s.WeatherPreview = "rain"
	s.Render(rain, 0)
	s.WeatherPreview = "storm"
	s.Render(storm, 0)
	if clear.Pix == rain.Pix || rain.Pix == storm.Pix || s.current.Code != 0 {
		t.Error("rain and storm previews should look different without replacing live conditions")
	}
}

func TestPreviewTime(t *testing.T) {
	rise := time.Date(2025, 6, 1, 6, 0, 0, 0, time.UTC)
	set := rise.Add(13 * time.Hour)
	for _, tc := range []struct {
		preview string
		elapsed time.Duration
		want    time.Time
	}{
		{"sunrise", 0, rise.Add(-75 * time.Minute)},
		{"sunrise", 75 * time.Second, rise},
		{"sunset", 149 * time.Second, set.Add(74 * time.Minute)},
		{"sunset", 150 * time.Second, set.Add(-75 * time.Minute)},
		{"night", 0, set.Add(31 * time.Minute)},
		{"night", 75 * time.Second, set.Add(31*time.Minute + 11*time.Hour/2)},
		{"night", 150 * time.Second, set.Add(31 * time.Minute)},
		{"", 0, rise},
	} {
		if got := previewTime(rise, rise, set, tc.preview, tc.elapsed); !got.Equal(tc.want) {
			t.Errorf("%s at %v: got %v, want %v", tc.preview, tc.elapsed, got, tc.want)
		}
	}
	first, _ := moonPosition(previewTime(rise, rise, set, "night", 0), rise, set)
	middle, _ := moonPosition(previewTime(rise, rise, set, "night", 75*time.Second), rise, set)
	last, _ := moonPosition(previewTime(rise, rise, set, "night", 149*time.Second), rise, set)
	if first >= middle || middle >= last {
		t.Errorf("night preview moon did not move across the sky: %d, %d, %d", first, middle, last)
	}
}

func TestMoonMovesThroughNight(t *testing.T) {
	rise := time.Date(2025, 6, 2, 6, 0, 0, 0, time.UTC)
	set := time.Date(2025, 6, 2, 18, 0, 0, 0, time.UTC)
	x1, y1 := moonPosition(set, rise, set)
	x2, y2 := moonPosition(set.Add(6*time.Hour), rise, set)
	x3, y3 := moonPosition(rise.Add(-time.Hour), rise, set)
	if !(x1 < x2 && x2 < x3 && y2 < y1 && y2 < y3) {
		t.Errorf("moon should rise, crest, and set: (%d,%d), (%d,%d), (%d,%d)", x1, y1, x2, y2, x3, y3)
	}
	xSet, ySet := moonPosition(rise.Add(29*time.Minute), rise, set)
	if xSet <= x3 || ySet < 23 {
		t.Errorf("moon should sink behind the horizon as the sun rises: (%d,%d)", xSet, ySet)
	}
	// Cached solar times from yesterday must also give the same pre-dawn position.
	x4, _ := moonPosition(rise.Add(-time.Hour), rise.AddDate(0, 0, -1), set.AddDate(0, 0, -1))
	if x4 != x3 {
		t.Errorf("stale solar times moved the pre-dawn moon: %d vs %d", x4, x3)
	}
}

func TestSunriseShowsSunAndSettingMoon(t *testing.T) {
	rise, _, _ := solarDay(time.Date(2025, 6, 2, 12, 0, 0, 0, time.UTC), 0, 0)
	now := rise.Add(10 * time.Minute)
	s, _ := New(Options{})
	s.now = func() time.Time { return now }
	s.checked = now
	c := matrix.NewCanvas()
	s.Render(c, time.Second)
	var sun, moon bool
	for y, row := range c.Pix {
		for x, p := range row {
			if x < 12 && p == (color.RGBA{255, 220, 55, 255}) {
				sun = true
			}
			if x > 50 && y < 24 && (p == (color.RGBA{245, 216, 123, 255}) || p == (color.RGBA{34, 53, 115, 255})) {
				moon = true
			}
		}
	}
	if !sun || !moon {
		t.Errorf("sunrise overlap: sun=%v, moon=%v", sun, moon)
	}
}

func TestMilkyWayOnlyInClearNight(t *testing.T) {
	now := time.Date(2025, 3, 20, 0, 0, 0, 0, time.UTC)
	s, _ := New(Options{})
	s.now = func() time.Time { return now }
	s.checked = now
	clear, storm := matrix.NewCanvas(), matrix.NewCanvas()
	s.Render(clear, time.Second)
	s.WeatherPreview = "storm"
	s.Render(storm, time.Second)
	if clear.Pix[4][5] != (color.RGBA{255, 230, 127, 255}) || storm.Pix[4][5] == clear.Pix[4][5] {
		t.Error("golden star field should appear at night and fade behind a storm")
	}
}

func TestCypressForeground(t *testing.T) {
	c := matrix.NewCanvas()
	c.Fill(color.RGBA{50, 100, 180, 255})
	drawTree(c, false)
	if c.Pix[6][11] == (color.RGBA{50, 100, 180, 255}) || c.Pix[30][7] == (color.RGBA{50, 100, 180, 255}) || c.Pix[10][30] != (color.RGBA{50, 100, 180, 255}) {
		t.Error("cypress should taper from a narrow tip to a wide foreground base")
	}
}

func TestLandscapeLayers(t *testing.T) {
	c := matrix.NewCanvas()
	drawGround(c, time.Second, true, 0, false)
	if c.Pix[19][30].A != 0 || c.Pix[24][30] == c.Pix[31][30] {
		t.Error("landscape needs open sky and distinct distant and near hills")
	}
}

func TestSceneWeather(t *testing.T) {
	loc := time.FixedZone("test", -6*3600)
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, loc)
	s, _ := New(Options{Lat: 41, Long: -87})
	s.now = func() time.Time { return now }
	s.fetch = func(context.Context, float64, float64) (Conditions, error) {
		return Conditions{Code: 95, Precipitation: 2}, nil
	}
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := matrix.NewCanvas()
	s.Render(c, time.Second)
	// Storm rain and the new landscape should both be present, without the pet.
	var blue, red bool
	for _, row := range c.Pix {
		for _, p := range row {
			blue = blue || p.B > 180 && p.B > p.R*2
			red = red || p.R > 200 && p.G < 100
		}
	}
	if !blue || red {
		t.Errorf("storm scene: rain=%v, unexpected pet red=%v", blue, red)
	}
}
