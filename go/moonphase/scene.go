// Package moonphase calculates the phase of the Moon and shows it as a
// matrix.Scene:
//
//	 ____    WAXING
//	/  ##\   GIBBOUS
//	|  ###|    87%
//	\  ##/     FULL
//	 ----     OCT 17
//
// The Sun's position comes from package prayer; the Moon's from the main
// terms of the lunar theory in Meeus' Astronomical Algorithms.
package moonphase

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"dotmatrix/matrix"
	"dotmatrix/planets"
)

// Scene implements matrix.Scene.
type Scene struct {
	// South mirrors the Moon as it is seen from the southern hemisphere,
	// where the waxing Moon is lit on the left.
	South    bool
	Location *time.Location // for the date of the next full or new moon

	now func() time.Time // overridable for tests
}

// New returns a Scene in the local time zone.
func New(south bool) *Scene { return &Scene{South: south, Location: time.Local} }

var (
	nameColor  = color.RGBA{220, 220, 220, 255}
	litColor   = color.RGBA{255, 170, 0, 255} // amber, like the prayer clock's prayer name
	eventColor = color.RGBA{130, 130, 130, 255}
	starColor  = color.RGBA{70, 70, 70, 255}
)

// A few fixed stars around the Moon.
var stars = [][2]int{{2, 3}, {29, 2}, {27, 29}, {1, 26}, {31, 15}}

// Moon placement: as big as the panel is tall, on the left.
const (
	moonX, moonY, moonR = 15.5, 16, 15
	textLeft            = 32 // the text is centred in the right half
)

// Render implements matrix.Scene. The phase changes slowly, so it redraws
// once a minute.
func (s *Scene) Render(c *matrix.Canvas, _ time.Duration) time.Duration {
	now := time.Now()
	if s.now != nil {
		now = s.now()
	}
	loc := s.Location
	if loc == nil {
		loc = time.Local
	}
	now = now.In(loc)
	p := At(now)

	c.Clear()
	for _, st := range stars {
		c.Set(st[0], st[1], starColor)
	}
	planets.DrawMoon(c, moonX, moonY, moonR, p.Elongation, s.South)

	name := strings.Fields(strings.ToUpper(p.Name())) // always two words
	event, at := "FULL", Next(now, FullMoon)
	if next := Next(now, NewMoon); next.Before(at) {
		event, at = "NEW", next
	}
	lines := []struct {
		y    int
		text string
		col  color.Color
	}{
		{1, name[0], nameColor},
		{7, name[1], nameColor},
		{14, percent(p.Illumination), litColor},
		{21, event, eventColor},
		{27, day(at.In(loc), now), eventColor},
	}
	for _, l := range lines {
		x := textLeft + (matrix.Width-textLeft-matrix.SmallTextWidth(l.text)+1)/2
		c.DrawSmallText(x, l.y, l.text, l.col)
	}
	return now.Truncate(time.Minute).Add(time.Minute).Sub(now)
}

// percent formats the lit fraction, never rounding a sliver to 0% or a
// nearly full Moon to 100%.
func percent(f float64) string {
	n := int(f*100 + 0.5)
	switch {
	case n == 0 && f > 0:
		n = 1
	case n == 100 && f < 1:
		n = 99
	}
	return fmt.Sprintf("%d%%", n)
}

// day names the day of t relative to now: TODAY, TOMORROW or e.g. OCT 17.
func day(t, now time.Time) string {
	y1, m1, d1 := now.Date()
	y2, m2, d2 := t.Date()
	today := time.Date(y1, m1, d1, 0, 0, 0, 0, time.UTC)
	switch time.Date(y2, m2, d2, 0, 0, 0, 0, time.UTC).Sub(today) / (24 * time.Hour) {
	case 0:
		return "TODAY"
	case 1:
		return "TOMORROW"
	}
	return strings.ToUpper(t.Format("Jan 2"))
}
