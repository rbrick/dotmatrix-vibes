package prayerclock

import (
	"testing"
	"time"

	"dotmatrix/matrix"
)

func newYork(t *testing.T) *Clock {
	c, err := New(40.7128, -74.0060, "ISNA", false)
	if err != nil {
		t.Fatal(err)
	}
	c.Location, _ = time.LoadLocation("America/New_York")
	return c
}

func TestNextPrayerAndTimeOfDay(t *testing.T) {
	c := newYork(t)
	at := func(h, m int) time.Time { return time.Date(2026, 10, 13, h, m, 0, 0, c.Location) }
	for _, tc := range []struct {
		now        time.Time
		next, when string
		emoji      string
	}{
		{at(3, 0), "Fajr", "5:50AM", ":moon:"},
		{at(6, 30), "Dhuhr", "12:42PM", ":sunrise:"},
		{at(10, 0), "Dhuhr", "12:42PM", ":sun:"},
		{at(13, 0), "Asr", "3:50PM", ":sun:"},
		{at(16, 0), "Maghrib", "6:19PM", ":afternoon:"},
		{at(18, 30), "Isha", "7:34PM", ":sunset:"},
		{at(22, 0), "Fajr", "5:51AM", ":moon:"}, // tomorrow's Fajr
	} {
		name, when := c.NextPrayer(tc.now)
		if name != tc.next || c.formatTime(when) != tc.when {
			t.Errorf("%s: next = %s %s, want %s %s", tc.now.Format("15:04"), name, c.formatTime(when), tc.next, tc.when)
		}
		if e := timeOfDay(c.day(tc.now), tc.now); e != tc.emoji {
			t.Errorf("%s: emoji = %s, want %s", tc.now.Format("15:04"), e, tc.emoji)
		}
	}
	if _, when := c.NextPrayer(at(22, 0)); when.Day() != 14 {
		t.Errorf("after Isha the next Fajr should be tomorrow, got %v", when)
	}
}

func TestRenderFitsAndRefreshesEachSecond(t *testing.T) {
	c := newYork(t)
	c.now = func() time.Time { return time.Date(2026, 10, 13, 12, 59, 20, 250e6, c.Location) }
	cv := matrix.NewCanvas()
	if next := c.Render(cv, 0); next != 750*time.Millisecond {
		t.Errorf("next redraw in %v, want 750ms", next)
	}
	// Widest possible lines.
	if w := matrix.TextWidth(":afternoon:") + emojiGap + matrix.TextWidth("12:59:59PM"); w > matrix.Width {
		t.Errorf("clock line is %dpx wide", w)
	}
	for _, s := range []string{"MAGHRIB", "12:59PM", formatCountdown(23*time.Hour + 59*time.Minute + 59*time.Second)} {
		if w := matrix.TextWidth(s); w > matrix.Width {
			t.Errorf("%q is %dpx wide", s, w)
		}
	}
	for d, want := range map[time.Duration]string{
		2*time.Hour + 5*time.Minute + 9*time.Second: "2h05m09s",
		4*time.Minute + 3*time.Second:               "4m03s",
	} {
		if got := formatCountdown(d); got != want {
			t.Errorf("countdown(%v) = %q, want %q", d, got, want)
		}
	}
}
