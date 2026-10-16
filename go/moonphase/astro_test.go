package moonphase

import (
	"testing"
	"time"
)

// Published times of principal phases (UTC, from the US Naval Observatory).
var phases = []struct {
	target float64
	at     string
}{
	{LastQuarter, "2024-01-04T03:30:00Z"},
	{NewMoon, "2024-01-11T11:57:00Z"},
	{FirstQuarter, "2024-01-18T03:53:00Z"},
	{FullMoon, "2024-01-25T17:54:00Z"},
	{NewMoon, "2024-04-08T18:21:00Z"},
	{FullMoon, "2024-09-18T02:34:00Z"},
	{NewMoon, "2025-03-29T10:58:00Z"},
}

func TestNextAndPreviousMatchPublishedPhases(t *testing.T) {
	for _, p := range phases {
		want, _ := time.Parse(time.RFC3339, p.at)
		for _, from := range []time.Time{want.Add(-20 * 24 * time.Hour), want.Add(-3 * time.Hour)} {
			if got := Next(from, p.target); absDur(got.Sub(want)) > 15*time.Minute {
				t.Errorf("Next(%v, %v) = %v, want %v", from, p.target, got, want)
			}
		}
		for _, from := range []time.Time{want.Add(3 * time.Hour), want.Add(25 * 24 * time.Hour)} {
			if got := Previous(from, p.target); absDur(got.Sub(want)) > 15*time.Minute {
				t.Errorf("Previous(%v, %v) = %v, want %v", from, p.target, got, want)
			}
		}
	}
}

func TestNextIsAlwaysAfter(t *testing.T) {
	newMoon, _ := time.Parse(time.RFC3339, "2024-01-11T11:57:00Z")
	exact := Next(newMoon.Add(-time.Hour), NewMoon)
	if got := Next(exact, NewMoon); got.Sub(exact) < 29*24*time.Hour {
		t.Errorf("Next from the exact new moon = %v, want the following one", got)
	}
}

func TestPhaseNamesAndIllumination(t *testing.T) {
	for _, c := range []struct {
		at, name string
		lit      float64
	}{
		{"2024-01-11T11:57:00Z", "New Moon", 0},
		{"2024-01-14T12:00:00Z", "Waxing Crescent", 0.1},
		{"2024-01-18T03:53:00Z", "First Quarter", 0.5},
		{"2024-01-21T12:00:00Z", "Waxing Gibbous", 0.85},
		{"2024-01-25T17:54:00Z", "Full Moon", 1},
		{"2024-01-29T00:00:00Z", "Waning Gibbous", 0.9},
		{"2024-02-02T23:18:00Z", "Last Quarter", 0.5},
		{"2024-02-06T00:00:00Z", "Waning Crescent", 0.15},
	} {
		at, _ := time.Parse(time.RFC3339, c.at)
		p := At(at)
		if p.Name() != c.name || p.Illumination < c.lit-0.08 || p.Illumination > c.lit+0.08 {
			t.Errorf("%s: %s, %.0f%% lit; want %s, about %.0f%%", c.at, p.Name(), p.Illumination*100, c.name, c.lit*100)
		}
	}
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
