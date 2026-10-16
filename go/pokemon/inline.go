package pokemon

import (
	"context"
	"image"
	"strings"

	"dotmatrix/matrix"
)

// InlineImages finds Pokémon shortcodes in text, such as :pikachu:,
// :mr-mime: or :shiny-gyarados:, and Poké Balls such as :pokeball: or
// :masterball:, and fetches their sprites for matrix.Text.Images. They are drawn still, at their native size (shrunk
// only if taller than the panel). Built-in emoji take precedence, numbers
// are ignored so "12:30:45" stays a time, and unknown names stay as text.
// The error reports a sprite that could not be fetched.
func InlineImages(ctx context.Context, src *Source, text string) (matrix.Images, error) {
	images := matrix.Images{}
	var firstErr error
	matrix.ScanShortcodes(text, func(name string) bool {
		if _, ok := images[name]; ok {
			return true
		}
		var img image.Image
		var err error
		if b, ok := parseBallShortcode(name); ok {
			img, err = src.BallSprite(ctx, b)
		} else if e, shiny, ok := parseShortcode(name); ok {
			img, err = src.Sprite(ctx, e, shiny)
		} else {
			return false
		}
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return false
		}
		images[name] = fit(crop(img))
		return true
	})
	return images, firstErr
}

// parseBallShortcode accepts ball names ending in "ball", so :moonball: is
// the Moon Ball while :moon: stays the moon emoji.
func parseBallShortcode(name string) (Ball, bool) {
	if !strings.HasSuffix(key(name), "ball") {
		return Ball{}, false
	}
	b, err := LookupBall(name)
	return b, err == nil
}

func parseShortcode(name string) (e Entry, shiny bool, ok bool) {
	n := strings.TrimSpace(strings.ToLower(name))
	n, shiny = strings.CutPrefix(n, "shiny-")
	if strings.Trim(n, "#0123456789") == "" {
		return Entry{}, false, false // empty or a number
	}
	e, err := Lookup(n)
	return e, shiny, err == nil
}
