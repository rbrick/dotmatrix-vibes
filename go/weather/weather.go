// Package weather fetches Open-Meteo conditions and renders them as a full-panel scene.
package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const DefaultInterval = 10 * time.Minute

// Options identifies the place shown by a Scene.
type Options struct {
	Lat, Long      float64
	Interval       time.Duration
	Preview        string // "", "sunrise", "sunset" or "night"
	WeatherPreview string // "", "rain" or "storm"
}

// Conditions is the part of an Open-Meteo response used by the scene.
type Conditions struct {
	Code          int
	Precipitation float64
}

// Scene is a live weather landscape.
type Scene struct {
	Options
	mu      sync.Mutex
	current Conditions
	err     error
	checked time.Time
	loading bool
	fetch   func(context.Context, float64, float64) (Conditions, error)
	now     func() time.Time
}

func New(opts Options) (*Scene, error) {
	if opts.Lat < -90 || opts.Lat > 90 || opts.Long < -180 || opts.Long > 180 {
		return nil, fmt.Errorf("weather: invalid latitude or longitude")
	}
	if opts.Interval == 0 {
		opts.Interval = DefaultInterval
	}
	if opts.Interval < time.Minute {
		return nil, fmt.Errorf("weather: refresh must be at least 1 minute")
	}
	switch opts.Preview {
	case "", "sunrise", "sunset", "night":
	default:
		return nil, fmt.Errorf("weather: preview must be sunrise, sunset or night")
	}
	switch opts.WeatherPreview {
	case "", "rain", "storm":
	default:
		return nil, fmt.Errorf("weather: weatherPreview must be rain or storm")
	}
	return &Scene{Options: opts, fetch: Fetch, now: time.Now}, nil
}

// Fetch gets current weather only. Solar position is calculated locally.
func Fetch(ctx context.Context, lat, long float64) (Conditions, error) {
	q := url.Values{
		"latitude": {fmt.Sprint(lat)}, "longitude": {fmt.Sprint(long)},
		"current": {"weather_code,precipitation"},
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.open-meteo.com/v1/forecast?"+q.Encode(), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Conditions{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Conditions{}, fmt.Errorf("open-meteo: %s", resp.Status)
	}
	var data struct {
		Current *struct {
			Code   int     `json:"weather_code"`
			Precip float64 `json:"precipitation"`
		} `json:"current"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return Conditions{}, fmt.Errorf("open-meteo: %w", err)
	}
	if data.Current == nil {
		return Conditions{}, fmt.Errorf("open-meteo: missing current weather")
	}
	return Conditions{data.Current.Code, data.Current.Precip}, nil
}

// Refresh updates the scene immediately.
func (s *Scene) Refresh(ctx context.Context) error {
	c, err := s.fetch(ctx, s.Lat, s.Long)
	s.mu.Lock()
	if err == nil {
		s.current = c
	}
	s.err, s.checked = err, s.now()
	s.mu.Unlock()
	return err
}

func (s *Scene) refreshIfDue() {
	s.mu.Lock()
	due := !s.loading && (s.checked.IsZero() || s.now().Sub(s.checked) >= s.Interval)
	if due {
		s.loading = true
	}
	s.mu.Unlock()
	if !due {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.Refresh(ctx)
		s.mu.Lock()
		s.loading = false
		s.mu.Unlock()
	}()
}
