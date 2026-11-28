package matrix

import (
	"image"
	"image/color"
	"sort"
	"strings"
	"unicode/utf8"
)

// Emoji are 7x7 multi-colour sprites that sit in a line of text like any other
// character. They keep their own colours regardless of the text colour.
//
// In sprite art, '.' is transparent and every other character is a colour
// from emojiPalette.
var emojiPalette = map[byte]color.RGBA{
	'R': {255, 0, 0, 255},     // red
	'O': {255, 100, 0, 255},   // orange
	'Y': {255, 200, 0, 255},   // yellow
	'G': {0, 200, 0, 255},     // green
	'B': {0, 80, 255, 255},    // blue
	'L': {100, 180, 255, 255}, // light blue
	'P': {160, 0, 255, 255},   // purple
	'p': {255, 80, 160, 255},  // pink
	'W': {255, 255, 255, 255}, // white
	'K': {0, 0, 0, 255},       // black
	'S': {255, 170, 90, 255},  // skin
	'b': {120, 60, 0, 255},    // brown
	'n': {170, 95, 30, 255},   // light brown
	'T': {255, 220, 140, 255}, // tan
	'g': {130, 130, 140, 255}, // grey
}

type emoji struct {
	name  string   // shortcode without colons, e.g. "heart" for :heart:
	runes []rune   // characters that produce this sprite
	art   []string // glyphHeight rows of emojiWidth characters
}

const emojiWidth = 7

// recolor returns heart-shaped art in a different colour.
func recolor(art []string, from, to string) []string {
	out := make([]string, len(art))
	for i, row := range art {
		out[i] = strings.ReplaceAll(row, from, to)
	}
	return out
}

var heartArt = []string{
	".RR.RR.",
	"RWRRRRR",
	"RRRRRRR",
	"RRRRRRR",
	".RRRRR.",
	"..RRR..",
	"...R...",
}

var emojis = []emoji{
	{"heart", []rune{'❤', '♥', '♡'}, heartArt},
	{"orange_heart", []rune{'🧡'}, recolor(heartArt, "R", "O")},
	{"yellow_heart", []rune{'💛'}, recolor(heartArt, "R", "Y")},
	{"green_heart", []rune{'💚'}, recolor(heartArt, "R", "G")},
	{"blue_heart", []rune{'💙'}, recolor(heartArt, "R", "B")},
	{"purple_heart", []rune{'💜'}, recolor(heartArt, "R", "P")},
	{"pink_heart", []rune{'🩷', '💗', '💖', '💕'}, recolor(heartArt, "R", "p")},
	{"star", []rune{'⭐', '★', '☆', '🌟'}, []string{
		"...Y...",
		"..YYY..",
		"YYYYYYY",
		".YYYYY.",
		"..YYY..",
		".YY.YY.",
		".Y...Y.",
	}},
	{"smile", []rune{'🙂', '😀', '😃', '😄', '😊', '☺'}, []string{
		".YYYYY.",
		"YYYYYYY",
		"YYKYKYY",
		"YYYYYYY",
		"YKYYYKY",
		"YYKKKYY",
		".YYYYY.",
	}},
	{"wink", []rune{'😉'}, []string{
		".YYYYY.",
		"YYYYYYY",
		"YYKYKKY",
		"YYYYYYY",
		"YKYYYKY",
		"YYKKKYY",
		".YYYYY.",
	}},
	{"sad", []rune{'🙁', '☹', '😢', '😞'}, []string{
		".YYYYY.",
		"YYYYYYY",
		"YYKYKYY",
		"YYYYYYY",
		"YYKKKYY",
		"YKYYYKY",
		".YYYYY.",
	}},
	{"heart_eyes", []rune{'😍', '🥰'}, []string{
		".YYYYY.",
		"YYYYYYY",
		"YRRYRRY",
		"YYRYRYY",
		"YKYYYKY",
		"YYKKKYY",
		".YYYYY.",
	}},
	{"cool", []rune{'😎'}, []string{
		".YYYYY.",
		"YYYYYYY",
		"KKKKKKK",
		"YKKYKKY",
		"YYYYYYY",
		"YKYYYKY",
		".YKKKY.",
	}},
	{"fire", []rune{'🔥'}, []string{
		"...R...",
		"..RR...",
		".RROR..",
		".ROORR.",
		"ROOYOOR",
		"ROYYYOR",
		".RYYYR.",
	}},
	{"thumbsup", []rune{'👍'}, []string{
		"...S...",
		"..SS...",
		"..SS...",
		"SSSSSS.",
		"BSSSSSS",
		"BSSSSS.",
		"BSSSSS.",
	}},
	{"sun", []rune{'☀', '🌞'}, []string{
		"Y..Y..Y",
		".YYYYY.",
		".YOOOY.",
		"YYOOOYY",
		".YOOOY.",
		".YYYYY.",
		"Y..Y..Y",
	}},
	{"sunrise", []rune{'🌅', '🌄'}, []string{
		"Y..Y..Y",
		".Y...Y.",
		"..YYY..",
		".YYYYY.",
		"YYYYYYY",
		"BBBBBBB",
		".B.B.B.",
	}},
	{"afternoon", []rune{'🌤', '⛅', '🌥'}, []string{
		".Y.Y...",
		"..YYY..",
		"YYYYY..",
		"..YWWW.",
		".YWWWWW",
		".WWWWWW",
		".......",
	}},
	{"sunset", []rune{'🌇', '🌆'}, []string{
		".......",
		".......",
		"..ROR..",
		".ROOOR.",
		"ROOOOOR",
		"PPPPPPP",
		".P.P.P.",
	}},
	{"moon", []rune{'🌙', '☾'}, []string{
		"..YYY..",
		".YY....",
		"YY.....",
		"YY.....",
		"YY.....",
		".YY....",
		"..YYY..",
	}},
	{"zap", []rune{'⚡'}, []string{
		"...YY..",
		"..YY...",
		".YY....",
		"YYYYYY.",
		"...YY..",
		"..YY...",
		".YY....",
	}},
	{"ghost", []rune{'👻'}, []string{
		".WWWWW.",
		"WWWWWWW",
		"WKWWWKW",
		"WWWWWWW",
		"WWWWWWW",
		"WWWWWWW",
		"W.W.W.W",
	}},
	{"invader", []rune{'👾'}, []string{
		".G...G.",
		"..GGG..",
		".GGGGG.",
		"GG.G.GG",
		"GGGGGGG",
		"G.GGG.G",
		"..G.G..",
	}},
	{"rainbow", []rune{'🌈'}, []string{
		".......",
		"..RRR..",
		".ROOOR.",
		"ROYYYOR",
		"OYGGGYO",
		"YGB.BGY",
		"GB...BG",
	}},
	{"tree", []rune{'🎄', '🌲'}, []string{
		"...Y...",
		"..GGG..",
		".GGRGG.",
		"..GGG..",
		".GGGBG.",
		"GGRGGGG",
		"...b...",
	}},
	{"middle_finger", []rune{'🖕'}, []string{
		"...S...",
		"...S...",
		"...S...",
		".SbSbS.",
		"SSSSSSS",
		".SSSSSS",
		"..SSSS.",
	}},
	{"check", []rune{'✅', '✔', '✓'}, []string{
		".......",
		"......G",
		".....GG",
		"G...GG.",
		"GG.GG..",
		".GGG...",
		"..G....",
	}},
	{"x", []rune{'❌', '✖', '✗'}, []string{
		"RR...RR",
		".RR.RR.",
		"..RRR..",
		"...R...",
		"..RRR..",
		".RR.RR.",
		"RR...RR",
	}},
	{"music", []rune{'🎵', '🎶', '♪', '♫'}, []string{
		"...LLLL",
		"...L..L",
		"...L..L",
		"...L..L",
		".LLL.LL",
		"LLLLLLL",
		".LL..L.",
	}},
	{"joy", []rune{'😂', '🤣'}, []string{
		".YYYYY.",
		"YYYYYYY",
		"LKKYKKL",
		"LYYYYYL",
		"LKKKKKL",
		"YYKRKYY",
		".YYYYY.",
	}},
	{"poop", []rune{'💩'}, []string{
		"...n...",
		"..nnn..",
		".nnnnn.",
		".WKnWK.",
		"nnnnnnn",
		"nWWWWWn",
		".nnnnn.",
	}},
	{"planet", []rune{'🪐'}, []string{
		".......",
		"..OOO..",
		".OOOOO.",
		"TOOOOOT",
		".TTTTT.",
		".OOOOO.",
		"..OOO..",
	}},
	{"telescope", []rune{'🔭'}, []string{
		".....LW",
		"....LLL",
		"...LLL.",
		"..LLL..",
		".W.g...",
		"..g.g..",
		".g...g.",
	}},
	{"drop", []rune{'💧'}, []string{
		"...L...",
		"..LLL..",
		".LLLLL.",
		"LLWLLLL",
		"LWLLLLL",
		"LLLLLLL",
		".LLLLL.",
	}},
}

var (
	emojiByRune = map[rune]*emoji{}
	emojiByName = map[string]*emoji{}
)

// Extra shortcodes for existing emoji.
var emojiAliases = map[string]string{
	"flipoff":         "middle_finger",
	"poo":             "poop",
	"laughing_crying": "joy",
	"lol":             "joy",
	"ringed_planet":   "planet",
}

func init() {
	for i := range emojis {
		e := &emojis[i]
		emojiByName[e.name] = e
		for _, r := range e.runes {
			emojiByRune[r] = e
		}
	}
	for alias, name := range emojiAliases {
		emojiByName[alias] = emojiByName[name]
	}
}

// EmojiInfo describes one built-in emoji.
type EmojiInfo struct {
	Name string // shortcode without colons
	Char string // the canonical emoji character
	// Pixels is the sprite, row by row: "#rrggbb" for each lit pixel and ""
	// for transparent ones.
	Pixels [][]string
}

// Emojis lists the built-in emoji in alphabetical order of name.
func Emojis() []EmojiInfo {
	out := make([]EmojiInfo, len(emojis))
	for i, e := range emojis {
		out[i] = EmojiInfo{Name: e.name, Char: string(e.runes[0]), Pixels: e.pixels()}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// isEmojiModifier reports runes that only modify the emoji before them:
// variation selectors, skin tones and zero-width joiners. They are skipped.
func isEmojiModifier(r rune) bool {
	return r == 0xfe0e || r == 0xfe0f || r == 0x200d || (r >= 0x1f3fb && r <= 0x1f3ff)
}

// cell is one drawable item of text: a font glyph, an emoji or an image.
type cell struct {
	glyph        *[glyphWidth]byte
	first, width int         // the glyph columns to draw
	col          color.Color // a glyph's inline colour; nil draws it in the text's colour
	emoji        *emoji
	img          image.Image
}

func (c cell) advance() int {
	switch {
	case c.emoji != nil:
		return emojiWidth + 1
	case c.img != nil:
		return c.img.Bounds().Dx() + 1
	}
	return c.width + 1
}

func (c cell) height() int {
	if c.img != nil {
		return c.img.Bounds().Dy()
	}
	return glyphHeight
}

// Images supplies pictures for :shortcodes: beyond the built-in emoji,
// keyed by the name between the colons. They are drawn at their own size,
// and a line of text grows to fit the tallest one.
type Images map[string]image.Image

// IsEmojiShortcode reports whether :name: is a built-in emoji.
func IsEmojiShortcode(name string) bool {
	_, ok := emojiByName[name]
	return ok
}

// ScanShortcodes walks s the same way text drawing does and calls resolve
// for each :name: that is not a built-in emoji. If resolve returns true the
// name is treated as an image shortcode (and its colons consumed); fill an
// Images map with exactly those names.
func ScanShortcodes(s string, resolve func(name string) bool) {
	for len(s) > 0 {
		if name, rest, ok := shortcodeAt(s); ok && (IsEmojiShortcode(name) || resolve(name) || isColorCode(name)) {
			s = rest
			continue
		}
		_, size := utf8.DecodeRuneInString(s)
		s = s[size:]
	}
}

// shortcodeAt reports whether s starts with :name:, and what follows it.
func shortcodeAt(s string) (name, rest string, ok bool) {
	if len(s) == 0 || s[0] != ':' {
		return "", s, false
	}
	end := strings.IndexByte(s[1:], ':')
	if end <= 0 || strings.ContainsAny(s[1:1+end], " \t\r\n") {
		return "", s, false // shortcodes never contain spaces: "12:30 :heart:"
	}
	return s[1 : 1+end], s[end+2:], true
}

// typographic maps punctuation that phones and word processors substitute
// automatically (curly quotes, dashes, ellipses) to the ASCII the font has,
// so "don’t" doesn't come out as "don?t".
var typographic = map[rune]string{
	'\u2018': "'", '\u2019': "'", '\u201a': "'", '\u201b': "'", '\u2032': "'", // ‘ ’ ‚ ‛ ′
	'\u201c': `"`, '\u201d': `"`, '\u201e': `"`, '\u201f': `"`, '\u2033': `"`, // “ ” „ ‟ ″
	'\u00ab': `"`, '\u00bb': `"`, // « »
	'\u2010': "-", '\u2011': "-", '\u2012': "-", '\u2013': "-", '\u2014': "-", '\u2212': "-", // ‐ ‑ ‒ – — −
	'\u2026': "...",                             // …
	'\u00a0': " ", '\u202f': " ", '\u2009': " ", // no-break and thin spaces
}

// narrowGlyphs are drawn without their blank side columns, so that "12:59:59"
// or "3.5" don't waste space around thin punctuation.
const narrowGlyphs = ":.,;!'|"

func glyphCell(r rune) cell {
	g := glyph(r)
	if !strings.ContainsRune(narrowGlyphs, r) {
		return cell{glyph: g, width: glyphWidth}
	}
	first, last := 0, glyphWidth-1
	for g[first] == 0 {
		first++
	}
	for g[last] == 0 {
		last--
	}
	return cell{glyph: g, first: first, width: last - first + 1}
}

// colorCode reads an inline colour code: :pink: or another colour name,
// :#ff8000: or :#f80:, or :default: for the text's own colour (nil). Hex codes
// need the '#' so that words like :add: or :bed: stay text.
func colorCode(name string) (col color.Color, ok bool) {
	name = strings.ToLower(name)
	if name == "default" {
		return nil, true
	}
	if c, ok := namedColors[name]; ok {
		return c, true
	}
	if strings.HasPrefix(name, "#") {
		if c, err := ParseColor(name); err == nil {
			return c, true
		}
	}
	return nil, false
}

func isColorCode(name string) bool {
	_, ok := colorCode(name)
	return ok
}

// cells splits text into drawable items, turning emoji characters and known
// :shortcodes: into sprites or images.
func cells(s string, images Images) []cell {
	out, _ := cellsFrom(s, images, nil)
	return out
}

// cellsFrom is cells with inline colour codes: glyphs start in col (nil for
// the text's colour), each :colour: code changes it for the glyphs after it,
// and the colour in effect at the end is returned so the next line can carry
// it on. Emoji and images keep their own colours. Built-in emoji and images
// win over colour names that clash with them.
func cellsFrom(s string, images Images, col color.Color) ([]cell, color.Color) {
	var out []cell
	for len(s) > 0 {
		if name, rest, ok := shortcodeAt(s); ok {
			if e, ok := emojiByName[name]; ok {
				out = append(out, cell{emoji: e})
				s = rest
				continue
			}
			if img, ok := images[name]; ok {
				out = append(out, cell{img: img})
				s = rest
				continue
			}
			if c, ok := colorCode(name); ok {
				col = c
				s = rest
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		switch e, ok := emojiByRune[r]; {
		case ok:
			out = append(out, cell{emoji: e})
		case isEmojiModifier(r):
		default:
			plain, ok := typographic[r]
			if !ok {
				plain = string(r)
			}
			for _, r := range plain {
				g := glyphCell(r)
				g.col = col
				out = append(out, g)
			}
		}
	}
	return out, col
}

func (e *emoji) pixels() [][]string {
	rows := make([][]string, len(e.art))
	for y, line := range e.art {
		rows[y] = make([]string, len(line))
		for x := 0; x < len(line); x++ {
			if px, ok := emojiPalette[line[x]]; ok {
				rows[y][x] = HexColor(px)
			}
		}
	}
	return rows
}

func (e *emoji) draw(c *Canvas, x, y int) {
	for row, line := range e.art {
		for col := 0; col < len(line); col++ {
			if px, ok := emojiPalette[line[col]]; ok {
				c.Set(x+col, y+row, px)
			}
		}
	}
}
