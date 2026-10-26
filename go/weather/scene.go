package weather

import (
	"image/color"
	"math"
	"time"

	"dotmatrix/matrix"
	"dotmatrix/moonphase"
)

func mix(a, b color.RGBA, n float64) color.RGBA {
	if n < 0 {
		n = 0
	}
	if n > 1 {
		n = 1
	}
	f := func(x, y uint8) uint8 { return uint8(float64(x)*(1-n) + float64(y)*n) }
	return color.RGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 255}
}

// Render draws a layered sky and landscape with animated weather.
func (s *Scene) Render(c *matrix.Canvas, elapsed time.Duration) time.Duration {
	s.refreshIfDue()
	s.mu.Lock()
	w := s.current
	s.mu.Unlock()
	switch s.WeatherPreview {
	case "rain":
		w.Code, w.Precipitation = 61, 2
	case "storm":
		w.Code, w.Precipitation = 95, 8
	}
	now := s.now()
	rise, set, daylightHours := solarDay(now, s.Lat, s.Long)
	now = previewTime(now, rise, set, s.Preview, elapsed)
	altitude, hourAngle := sunAngles(now, s.Lat, s.Long)

	// Twilight is deliberately broad, giving gradual warm sunrise/sunset transitions.
	day := altitude > -6
	warm := 0.0
	for _, edge := range []time.Time{rise, set} {
		if d := math.Abs(now.Sub(edge).Minutes()); d < 75 {
			warm = math.Max(warm, 1-d/75)
		}
	}
	top, bottom := color.RGBA{22, 32, 105, 255}, color.RGBA{32, 64, 128, 255}
	if day {
		top, bottom = color.RGBA{28, 77, 165, 255}, color.RGBA{60, 150, 205, 255}
	}
	top, bottom = mix(top, color.RGBA{80, 50, 127, 255}, warm), mix(bottom, color.RGBA{225, 127, 49, 255}, warm)
	for y := 0; y < matrix.Height; y++ {
		for x := 0; x < matrix.Width; x++ {
			col := mix(top, bottom, float64(y)/31)
			wave := math.Sin(float64(x)*.19 + float64(y)*.47 + 1.7*math.Sin(float64(x)*.09+elapsed.Seconds()*.13))
			if wave > .35 {
				col = mix(col, mix(color.RGBA{65, 190, 198, 255}, color.RGBA{242, 160, 64, 255}, warm), (wave-.35)*.42)
			}
			if curl := math.Sin(float64(x)*.16 - float64(y)*.65 - elapsed.Seconds()*.08); curl > .6 {
				col = mix(col, color.RGBA{83, 107, 202, 255}, (curl-.6)*.45)
			}
			if (x*7+y*13)%19 < 3 {
				col = mix(col, color.RGBA{181, 172, 107, 255}, .12)
			}
			c.Set(x, y, col)
		}
	}

	if !day && w.Code < 3 {
		drawMilkyWay(c, elapsed, w.Code == 0)
	}
	if !day || (!rise.IsZero() && now.Before(rise.Add(30*time.Minute))) {
		phase := moonphase.At(now)
		mx, my := moonPosition(now, rise, set)
		if phase.Illumination > .05 {
			drawGlow(c, mx, my, 6, color.RGBA{245, 193, 87, 255})
		}
		drawMoon(c, mx, my, phase.Elongation)
	}
	if day && w.Code < 2 {
		// Hour angle sets east-to-west travel; altitude sets the Sun's height.
		progress := (hourAngle + daylightHours*15 + 7.5) / (daylightHours*30 + 15)
		if daylightHours == 0 {
			progress = (hourAngle + 180) / 360 // polar day
		}
		x := 4 + int(math.Max(0, math.Min(1, progress))*55)
		y := 23 - int(math.Max(0, altitude+0.833)*20/90)
		drawGlow(c, x, y, 5, color.RGBA{250, 181, 65, 255})
		for yy := -3; yy <= 3; yy++ {
			for xx := -3; xx <= 3; xx++ {
				if xx*xx+yy*yy <= 9 {
					c.Set(x+xx, y+yy, color.RGBA{255, 220, 55, 255})
				}
			}
		}
	}

	cloudy := w.Code >= 1
	rain := (w.Code >= 51 && w.Code <= 67) || (w.Code >= 80 && w.Code <= 82) || w.Code >= 95 || w.Precipitation > 0
	storm := w.Code >= 95
	if cloudy {
		drawClouds(c, elapsed, w.Code >= 3)
	}
	if rain {
		drawRain(c, elapsed, w.Code >= 65 || w.Code >= 82)
	}
	drawGround(c, elapsed, day, warm, rain)
	drawTree(c, day)
	if storm && int(elapsed/(170*time.Millisecond))%29 == 0 {
		for y := 0; y < 28; y++ {
			for x := 0; x < 64; x++ {
				c.Set(x, y, mix(c.Pix[y][x], color.RGBA{255, 255, 255, 255}, .65))
			}
		}
	}
	return 100 * time.Millisecond
}

// Sunrise and sunset previews sweep across twilight in 150 seconds, then repeat.
func previewTime(now, rise, set time.Time, preview string, elapsed time.Duration) time.Time {
	if rise.IsZero() || set.IsZero() {
		return now
	}
	switch preview {
	case "sunrise":
		return rise.Add(-75*time.Minute + time.Duration(elapsed/time.Second%150)*time.Minute)
	case "sunset":
		return set.Add(-75*time.Minute + time.Duration(elapsed/time.Second%150)*time.Minute)
	case "night":
		start := set.Add(31 * time.Minute)
		end := rise.Add(24*time.Hour + 31*time.Minute)
		if end.After(start) {
			return start.Add(end.Sub(start) * (elapsed / time.Second % 150) / 150)
		}
		return set.Add(3 * time.Hour)
	default:
		return now
	}
}

// moonPosition follows the visible night from sunset to the next sunrise.
func moonPosition(now, sunrise, sunset time.Time) (int, int) {
	if sunrise.IsZero() || sunset.IsZero() {
		return 32, 7
	}
	if now.Before(sunrise.Add(30 * time.Minute)) {
		sunset = sunset.AddDate(0, 0, -1)
	} else {
		sunrise = sunrise.AddDate(0, 0, 1)
	}
	progress := now.Sub(sunset).Seconds() / sunrise.Add(30*time.Minute).Sub(sunset).Seconds()
	progress = math.Max(0, math.Min(1, progress))
	return 4 + int(progress*55), 14 - int(math.Sin(progress*math.Pi)*11) + int(math.Max(0, (progress-.9)*110))
}

// Layered cobalt and turquoise strokes form a slow, swirling Milky Way.
func drawMilkyWay(c *matrix.Canvas, elapsed time.Duration, clear bool) {
	shift := elapsed.Seconds() * .08
	for x := 0; x < matrix.Width; x++ {
		mid := 14 - x/8 + int(2*math.Sin(float64(x)*.15+shift)+math.Sin(float64(x)*.35-shift))
		for dy := -4; dy <= 4; dy++ {
			y := mid + dy
			if y < 0 || y >= 21 {
				continue
			}
			strength := (1 - math.Abs(float64(dy))/5) * .5
			if !clear {
				strength *= .55
			}
			col := color.RGBA{65, 157, 190, 255}
			if (x+dy+int(shift*4))%5 == 0 {
				col = color.RGBA{171, 188, 137, 255}
			}
			c.Set(x, y, mix(c.Pix[y][x], col, strength))
		}
	}
	for _, center := range [][2]int{{14, 12}, {40, 12}} {
		for i := 0; i < 45; i++ {
			a := float64(i)*.35 + shift
			r := float64(i) / 8
			x, y := center[0]+int(r*math.Cos(a)), center[1]+int(r*math.Sin(a))
			if x >= 0 && x < matrix.Width && y >= 0 && y < 21 {
				c.Set(x, y, mix(c.Pix[y][x], color.RGBA{97, 166, 186, 255}, .48))
			}
		}
	}
	stars := [][2]int{{5, 4}, {15, 13}, {25, 6}, {37, 14}, {48, 4}, {58, 11}}
	for i, p := range stars {
		drawGlow(c, p[0], p[1], 3, color.RGBA{237, 189, 69, 255})
		if (int(elapsed/(450*time.Millisecond))+i)%4 != 0 {
			c.Set(p[0], p[1], color.RGBA{255, 230, 127, 255})
		}
	}
}

func drawGlow(c *matrix.Canvas, cx, cy, radius int, col color.RGBA) {
	for yy := -radius; yy <= radius; yy++ {
		for xx := -radius; xx <= radius; xx++ {
			d := xx*xx + yy*yy
			if d > radius*radius || cx+xx < 0 || cx+xx >= matrix.Width || cy+yy < 0 || cy+yy >= matrix.Height {
				continue
			}
			strength := .35 * (1 - float64(d)/float64(radius*radius))
			c.Set(cx+xx, cy+yy, mix(c.Pix[cy+yy][cx+xx], col, strength))
		}
	}
}

func drawMoon(c *matrix.Canvas, cx, cy int, elongation float64) {
	// Eight chunky phases, Minecraft-style rather than astronomical sub-pixel shading.
	phase := int((elongation+22.5)/45) % 8
	for y := -4; y <= 4; y++ {
		for x := -4; x <= 4; x++ {
			if x*x+y*y <= 17 {
				lit := false
				switch phase {
				case 0:
					lit = false
				case 1:
					lit = x >= 2
				case 2:
					lit = x >= 0
				case 3:
					lit = x >= -2
				case 4:
					lit = true
				case 5:
					lit = x <= 2
				case 6:
					lit = x <= 0
				case 7:
					lit = x <= -2
				}
				if lit {
					c.Set(cx+x, cy+y, color.RGBA{245, 216, 123, 255})
				} else {
					c.Set(cx+x, cy+y, color.RGBA{34, 53, 115, 255})
				}
			}
		}
	}
}

func drawClouds(c *matrix.Canvas, elapsed time.Duration, heavy bool) {
	shade := color.RGBA{119, 167, 191, 255}
	if heavy {
		shade = color.RGBA{64, 85, 145, 255}
	}
	for i := 0; i < 3; i++ {
		// High clouds drift slowly and fade into the sky; low clouds pass faster.
		x := (i*27+int(elapsed/(time.Duration(900-i*250)*time.Millisecond)))%84 - 14
		y := 2 + i*4
		col := mix(shade, c.Pix[y][min(63, max(0, x+6))], float64(2-i)*.28)
		for yy := 0; yy < 4; yy++ {
			for xx := 0; xx < 13; xx++ {
				if yy >= 2 || (xx > 2 && xx < 7) || (xx > 7 && xx < 11) {
					stroke := mix(col, shade, float64(yy)*.14)
					if (x+xx+yy)%7 == 0 {
						stroke = mix(stroke, color.RGBA{179, 189, 148, 255}, .28)
					}
					c.Set(x+xx, y+yy, stroke)
				}
			}
		}
	}
}

func drawRain(c *matrix.Canvas, elapsed time.Duration, heavy bool) {
	n := 14
	if heavy {
		n = 24
	}
	tick := int(elapsed / (80 * time.Millisecond))
	for i := 0; i < n; i++ {
		x := (i*17 + tick*3) % 64
		y := 8 + (i*7+tick*2)%17
		c.Set(x, y, color.RGBA{107, 175, 200, 255})
		c.Set(x-1, y+1, color.RGBA{64, 118, 166, 255})
	}
}

// A tall, wind-bent cypress anchors the foreground against the moving sky.
func drawTree(c *matrix.Canvas, day bool) {
	shadow := color.RGBA{12, 29, 48, 255}
	highlight := color.RGBA{53, 85, 76, 255}
	if day {
		shadow = color.RGBA{23, 52, 58, 255}
		highlight = color.RGBA{69, 107, 80, 255}
	}
	for y := 6; y < matrix.Height; y++ {
		center := 11 + int(math.Sin(float64(y)*.3))
		width := min(5, (y-5)/4)
		if y > 14 && y < 19 {
			width++ // a flame-like branch curls away from the trunk
		}
		for dx := -width; dx <= width; dx++ {
			x := center + dx
			col := shadow
			if (y*5+dx*3)%9 < 3 {
				col = highlight
			}
			c.Set(x, y, col)
		}
	}
}

func drawGround(c *matrix.Canvas, elapsed time.Duration, day bool, warm float64, rain bool) {
	far := color.RGBA{32, 60, 112, 255}
	near := color.RGBA{19, 40, 76, 255}
	if day {
		far = color.RGBA{60, 111, 139, 255}
		near = color.RGBA{35, 79, 87, 255}
	}
	far = mix(far, color.RGBA{151, 100, 83, 255}, warm*.6)
	for x := 0; x < matrix.Width; x++ {
		// Two overlapping ridgelines give the 32-pixel sky a distant horizon.
		horizon := 22 + int(2*math.Sin(float64(x)*.13)+math.Sin(float64(x)*.3))
		front := 27 + int(2*math.Sin(float64(x)*.2+1))
		for y := horizon; y < matrix.Height; y++ {
			col := far
			if y >= front {
				col = near
			}
			if (x*3+y*11)%13 < 3 {
				col = mix(col, color.RGBA{127, 152, 103, 255}, .24)
			}
			c.Set(x, y, col)
		}
	}
	// A few wind-swayed foreground reeds put a third plane in front.
	for x := 2; x < matrix.Width; x += 7 {
		tip := x + int(math.Sin(elapsed.Seconds()*1.4+float64(x))*2)
		c.Set(tip, 27, mix(near, color.RGBA{204, 181, 80, 255}, .45))
		c.Set(x, 28, near)
	}
	if rain {
		// Near drops streak across the landscape as well as the sky.
		tick := int(elapsed / (70 * time.Millisecond))
		for i := 0; i < 7; i++ {
			x := (i*23 + tick*4) % 64
			y := 20 + (i*11+tick*3)%11
			c.Set(x, y, color.RGBA{125, 184, 205, 255})
			c.Set(x-1, y+1, color.RGBA{73, 130, 171, 255})
		}
	}
}
