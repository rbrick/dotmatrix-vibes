package orrery

import (
	"image/color"
	"math"
	"testing"
	"time"

	"dotmatrix/matrix"
	"dotmatrix/prayer"
)

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t.Add(12 * time.Hour)
}

func angle(a, b float64) float64 { return math.Abs(math.Remainder(a-b, 360)) }

// Earth is opposite the Sun as seen from Earth. The solar longitude is
// measured from the equinox of date, which precesses 1.397° a century from
// the J2000 equinox the planetary elements use.
func TestEarthMatchesSolarLongitude(t *testing.T) {
	for _, d := range []string{"2000-01-01", "2024-03-20", "2026-10-13", "2040-07-04"} {
		at := date(d)
		got := position(2, at).longitude()
		precession := 1.3970 * prayer.DaysSinceJ2000(at) / 36525
		want := math.Mod(prayer.SunEclipticLongitude(at)+180-precession, 360)
		if angle(got, want) > 0.05 {
			t.Errorf("%s: Earth at %.3f°, want %.3f°", d, got, want)
		}
	}
}

// At opposition (outer planets) or inferior conjunction (inner planets) a
// planet and Earth are in line with the Sun: same heliocentric longitude.
// The alignment must happen within a day of the published date.
func TestAlignments(t *testing.T) {
	for _, c := range []struct {
		planet int
		date   string
	}{
		{0, "2024-12-06"}, // Mercury inferior conjunction
		{1, "2025-03-23"}, // Venus inferior conjunction
		{3, "2025-01-16"}, // Mars opposition
		{4, "2024-12-07"}, // Jupiter opposition
		{5, "2024-09-08"}, // Saturn opposition
		{6, "2024-11-17"}, // Uranus opposition
		{7, "2024-09-21"}, // Neptune opposition
		{8, "2024-07-23"}, // Pluto opposition
	} {
		ahead := func(at time.Time) float64 {
			return math.Remainder(position(c.planet, at).longitude()-position(2, at).longitude(), 360)
		}
		before, after := ahead(date(c.date).Add(-36*time.Hour)), ahead(date(c.date).Add(36*time.Hour))
		if before*after > 0 || math.Abs(before) > 20 {
			t.Errorf("%s: no alignment around %s (%.2f° then %.2f°)", planets[c.planet].name, c.date, before, after)
		}
	}
}

func TestDistances(t *testing.T) {
	at := date("2026-10-13")
	for i, p := range planets {
		r := position(i, at).length()
		if r < p.el.a*(1-p.el.e)*0.99 || r > p.el.a*(1+p.el.e)*1.01 {
			t.Errorf("%s at %.2f AU, outside its orbit", p.name, r)
		}
	}
}

func TestScene(t *testing.T) {
	now := date("2026-10-13")
	s := New(Options{Label: true})
	s.now = func() time.Time { return now }
	c := matrix.NewCanvas()
	if next := s.Render(c, 0); next <= 0 || next > blink {
		t.Errorf("labelled scene redraws in %v, want within a blink", next)
	}
	// Every planet is drawn in its colour (or white while blinking) where
	// it is projected.
	for i := range planets {
		x, y, _ := project(position(i, now))
		got := c.Pix[int(math.Floor(y))][int(math.Floor(x))]
		if got != planetColors[i] && got != (color.RGBA{255, 255, 255, 255}) {
			t.Errorf("%s pixel = %v", planets[i].name, got)
		}
		if x < 0 || x >= matrix.Width || y < 0 || y >= matrix.Height {
			t.Errorf("%s off the panel at %.1f, %.1f", planets[i].name, x, y)
		}
	}
	// Pluto's whole orbit, including aphelion, fits on the panel.
	o := planets[8].orbitAt(now)
	for k := 0; k < 360; k++ {
		if x, y, _ := project(o.at(float64(k) * math.Pi / 180)); x < 0 || x >= matrix.Width || y < -1 || y >= matrix.Height+1 {
			t.Fatalf("Pluto's orbit leaves the panel at %.1f, %.1f", x, y)
		}
	}
	if next := New(Options{Speed: 10}).Render(c, time.Second); next != frameInterval {
		t.Errorf("time-lapse redraws in %v", next)
	}
}
