// Package pokemon shows Pokémon box sprites (the icons from the PC storage
// boxes) from PokéSprite, https://github.com/msikma/pokesprite, as a
// matrix.Scene. Sprites are downloaded when first needed and cached on disk.
package pokemon

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Entry is one Pokémon from the National Pokédex.
type Entry struct {
	Dex  int
	Name string // English name, e.g. "Nidoran♀"
	Slug string // PokéSprite file name, e.g. "nidoran-f"
}

//go:embed pokedex.tsv
var pokedexTSV string

// Pokedex lists every Pokémon PokéSprite has a box sprite for, by number.
var Pokedex = func() []Entry {
	var es []Entry
	for _, line := range strings.Split(strings.TrimSpace(pokedexTSV), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		dex, err := strconv.Atoi(f[0])
		if err != nil || len(f) != 3 {
			panic("pokemon: bad pokedex.tsv line: " + line)
		}
		es = append(es, Entry{dex, f[1], f[2]})
	}
	return es
}()

// key folds a name for matching: "Mr. Mime", "mr mime" and "MR-MIME" are
// all "mrmime".
func key(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case 'é':
			r = 'e'
		case '♀':
			b.WriteString("f")
			continue
		case '♂':
			b.WriteString("m")
			continue
		}
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Lookup finds a Pokémon by name (any case and punctuation) or Pokédex
// number ("25", "025" or "#25").
func Lookup(q string) (Entry, error) {
	q = strings.TrimSpace(q)
	if n, err := strconv.Atoi(strings.TrimPrefix(q, "#")); err == nil {
		if n >= 1 && n <= len(Pokedex) {
			return Pokedex[n-1], nil
		}
		return Entry{}, fmt.Errorf("pokemon: no Pokémon #%d (1-%d)", n, len(Pokedex))
	}
	k := key(q)
	for _, e := range Pokedex {
		if key(e.Name) == k || key(e.Slug) == k {
			return e, nil
		}
	}
	return Entry{}, fmt.Errorf("pokemon: no Pokémon called %q", q)
}

// Label is the name as the panel's small font can show it.
func (e Entry) Label() string {
	r := strings.NewReplacer("♀", "-F", "♂", "-M", "é", "E")
	return strings.ToUpper(r.Replace(e.Name))
}
