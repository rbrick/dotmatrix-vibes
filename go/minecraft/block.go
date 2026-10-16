package minecraft

import (
	"image"
	"image/color"
	"math"
)

// blockSize is the width and height of the grass block icon.
const blockSize = 22

// grassBlock draws an isometric grass block: a grassy top, and dirt sides
// with grass hanging over their top edges. Each face is an 8x8 texel grid,
// like a block texture at half resolution, with fixed pseudo-random
// shading. The left side is in light, the right in shadow.
func grassBlock() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, blockSize, blockSize))
	const s = blockSize / 2.0
	for y := 0; y < blockSize; y++ {
		for x := 0; x < blockSize; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			var col color.RGBA
			switch {
			case math.Abs(px-s)/s+math.Abs(py-s/2)/(s/2) <= 1: // top face
				// Face coordinates along the diamond's edges from its left
				// corner, 0-1: P = left + u*(s, -s/2) + v*(s, s/2).
				u := (px/s - (py-s/2)/(s/2)) / 2
				v := (px/s + (py-s/2)/(s/2)) / 2
				col = shade(grassTop, texel(u, v, 1), 1)
			case px < s && py-(s/2+px/2) <= s && py >= s/2+px/2: // left face
				col = side(px/s, (py-(s/2+px/2))/s, 2, 0.9)
			case px >= s && py-(s-(px-s)/2) <= s && py >= s-(px-s)/2: // right face
				col = side((px-s)/s, (py-(s-(px-s)/2))/s, 3, 0.72)
			default:
				continue
			}
			img.SetRGBA(x, y, col)
		}
	}
	return img
}

var (
	grassTop = []color.RGBA{{98, 160, 54, 255}, {86, 145, 46, 255}, {110, 172, 64, 255}, {78, 132, 40, 255}}
	// Dirt is lighter and more orange than the game's, because browns turn
	// red on the panel once their green rounds down.
	dirt = []color.RGBA{{170, 125, 70, 255}, {155, 112, 62, 255}, {185, 138, 80, 255}, {140, 100, 55, 255}}
)

// side shades a dirt face, u across and v down (0-1), with a ragged strip
// of grass along its top edge.
func side(u, v float64, seed uint32, light float64) color.RGBA {
	tu := int(u * 8)
	if v*8 < float64(2+hash(uint32(tu), 0, seed+10)%2) { // grass hangs 2-3 texels
		return shade(grassTop, texel(u, v, seed), light)
	}
	return shade(dirt, texel(u, v, seed), light)
}

// texel picks a palette index for the 8x8 texel containing (u, v).
func texel(u, v float64, seed uint32) int {
	tu, tv := min(int(u*8), 7), min(int(v*8), 7)
	return int(hash(uint32(tu), uint32(tv), seed) % 4)
}

func shade(palette []color.RGBA, i int, light float64) color.RGBA {
	c := palette[i]
	return color.RGBA{uint8(float64(c.R) * light), uint8(float64(c.G) * light), uint8(float64(c.B) * light), 255}
}

func hash(x, y, seed uint32) uint32 {
	h := x*0x8da6b343 ^ y*0xd8163841 ^ seed*0xcb1ab31f
	h ^= h >> 13
	h *= 0x5bd1e995
	return h ^ h>>15
}
