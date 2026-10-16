package pokemon

import (
	"fmt"
	"strings"
)

// Ball is one of the Poké Balls in PokéSprite's items/ball directory.
type Ball struct {
	Slug string // file name without .png, e.g. "poke" or "hisuian-great"
}

// Balls lists every Poké Ball sprite.
var Balls = func() []Ball {
	var bs []Ball
	for _, slug := range strings.Fields(`
		poke great ultra master premier
		beast cherish dive dream dusk fast friend heal heavy level love lure
		luxury moon nest net park quick repeat safari sport timer strange
		hisuian-poke hisuian-great hisuian-ultra hisuian-heavy
		feather wing jet leaden gigaton origin`) {
		bs = append(bs, Ball{slug})
	}
	return bs
}()

// Shortcode is the ball's name in text, without the colons: "pokeball",
// "greatball", "hisuian-pokeball".
func (b Ball) Shortcode() string { return b.Slug + "ball" }

// Name is the ball's name as the games write it, e.g. "Hisuian Poké Ball".
func (b Ball) Name() string {
	words := strings.Split(b.Slug, "-")
	for i, w := range words {
		if w == "poke" {
			words[i] = "Poké"
		} else {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ") + " Ball"
}

// LookupBall finds a ball by name, forgiving case, spaces, hyphens and
// accents: "Poké Ball", "pokeball", "poke-ball", "master" and
// "hisuian-pokeball" all work.
func LookupBall(q string) (Ball, error) {
	k := strings.TrimSuffix(key(q), "ball")
	for _, b := range Balls {
		if key(b.Slug) == k {
			return b, nil
		}
	}
	return Ball{}, fmt.Errorf("pokemon: no ball called %q", q)
}
