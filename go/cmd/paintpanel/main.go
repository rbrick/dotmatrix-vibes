// Command paintpanel puts a picture (a painting, a photo) on the panel. It
// crops the picture to the panel's 2:1 shape, or fits the whole picture and
// continues its edges to fill the sides, averages the pixels behind each LED,
// lifts dark tones the LEDs would show as black, and sends the result to
// matrixd. -preview saves what the LEDs will show, to check a crop first.
//
//	paintpanel -in mona.jpg -top 0.095 -preview p.png -n   # just preview
//	paintpanel -in mona.jpg -top 0.095 -save "Mona Lisa"     # show and save as a scene
//	paintpanel -in 'https://commons.wikimedia.org/wiki/Special:FilePath/Mona_Lisa.jpg?width=1200' -fit
//
// -top and -left are where the crop starts, as fractions of the picture's
// height and width, and -w is the crop's width as a fraction of the
// picture's width (its height follows from the panel's 2:1 shape).
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"dotmatrix/matrix"
)

type options struct {
	Top, Left, Width float64 // crop, as fractions of the picture
	Fit              bool    // the whole picture instead of a crop
	Gamma            float64 // < 1 lifts dark tones
	Saturation       float64 // > 1 strengthens colours
}

func main() {
	in := flag.String("in", "", "picture to show: a file, or an http(s) URL")
	var o options
	flag.Float64Var(&o.Top, "top", 0, "crop top, as a fraction of the picture's height")
	flag.Float64Var(&o.Left, "left", 0, "crop left, as a fraction of the picture's width")
	flag.Float64Var(&o.Width, "w", 1, "crop width, as a fraction of the picture's width")
	flag.BoolVar(&o.Fit, "fit", false, "show the whole picture, centred, continuing its edges to fill the panel")
	flag.Float64Var(&o.Gamma, "gamma", 0.8, "brightness curve; below 1 lifts dark tones")
	flag.Float64Var(&o.Saturation, "sat", 1.2, "saturation; above 1 strengthens colours")
	preview := flag.String("preview", "", "save what the LEDs will show to this PNG")
	dryRun := flag.Bool("n", false, "don't send to the panel")
	server := flag.String("server", "http://dotmatrix.local:8080", "matrixd address")
	save := flag.String("save", "", "also save the picture as a scene with this name")
	flag.Parse()
	if *in == "" {
		flag.Usage()
		os.Exit(2)
	}

	src, err := load(*in)
	if err != nil {
		log.Fatal(err)
	}
	c, err := convert(src, o)
	if err != nil {
		log.Fatal(err)
	}
	if *preview != "" {
		if err := writePNG(*preview, ledPreview(c)); err != nil {
			log.Fatal(err)
		}
	}
	if *dryRun {
		return
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, c); err != nil {
		log.Fatal(err)
	}
	if err := post(*server+"/api/image", "image/png", &buf); err != nil {
		log.Fatal(err)
	}
	if *save != "" {
		body, _ := json.Marshal(map[string]string{"name": *save})
		if err := post(*server+"/api/scenes", "application/json", bytes.NewReader(body)); err != nil {
			log.Fatal(err)
		}
	}
}

// load reads a picture from a file or an http(s) URL.
func load(in string) (image.Image, error) {
	var r io.Reader
	if strings.HasPrefix(in, "http://") || strings.HasPrefix(in, "https://") {
		req, err := http.NewRequest("GET", in, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "dotmatrix-paintpanel/1.0") // Wikimedia asks for one
		resp, err := (&http.Client{Timeout: time.Minute}).Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetching %s: %s", in, resp.Status)
		}
		r = resp.Body
	} else {
		f, err := os.Open(in)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	}
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", in, err)
	}
	return img, nil
}

// convert turns a picture into what the panel shows: each LED is the average
// colour of the part of the picture behind it, adjusted by Gamma and
// Saturation.
func convert(src image.Image, o options) (*matrix.Canvas, error) {
	b := src.Bounds()
	if b.Empty() {
		return nil, errors.New("empty picture")
	}
	W, H := float64(b.Dx()), float64(b.Dy())
	// The picture area behind the panel's outW×outH LEDs, placed at offX, offY.
	x0, y0, cw, ch := float64(b.Min.X), float64(b.Min.Y), W, H
	outW, outH, offX, offY := matrix.Width, matrix.Height, 0, 0
	if o.Fit {
		scale := math.Min(float64(matrix.Width)/W, float64(matrix.Height)/H)
		outW, outH = max(1, int(math.Round(W*scale))), max(1, int(math.Round(H*scale)))
		offX, offY = (matrix.Width-outW)/2, (matrix.Height-outH)/2
	} else {
		cw = W * o.Width
		ch = cw * matrix.Height / matrix.Width
		x0 += W * o.Left
		y0 += H * o.Top
		if o.Width <= 0 || o.Left < 0 || o.Top < 0 || x0+cw > float64(b.Max.X)+0.5 || y0+ch > float64(b.Max.Y)+0.5 {
			return nil, fmt.Errorf("crop is not inside the picture: with -w %g the crop is %.3f of the height tall, so -top can be at most %.3f and -left at most %.3f",
				o.Width, ch/H, 1-ch/H, 1-o.Width)
		}
	}

	c := matrix.NewCanvas()
	for oy := 0; oy < matrix.Height; oy++ {
		py := min(max(oy-offY, 0), outH-1) // beyond the picture, repeat its edge
		sy0 := int(y0 + ch*float64(py)/float64(outH))
		sy1 := max(sy0+1, int(y0+ch*float64(py+1)/float64(outH)))
		for ox := 0; ox < matrix.Width; ox++ {
			px := min(max(ox-offX, 0), outW-1)
			sx0 := int(x0 + cw*float64(px)/float64(outW))
			sx1 := max(sx0+1, int(x0+cw*float64(px+1)/float64(outW)))
			var r, g, bl, n float64
			for y := sy0; y < min(sy1, b.Max.Y); y++ {
				for x := sx0; x < min(sx1, b.Max.X); x++ {
					cr, cg, cb, _ := src.At(x, y).RGBA()
					r += float64(cr >> 8)
					g += float64(cg >> 8)
					bl += float64(cb >> 8)
					n++
				}
			}
			if n > 0 {
				r, g, bl = r/n, g/n, bl/n
			}
			c.Pix[oy][ox] = adjust(r, g, bl, o)
		}
	}
	return c, nil
}

func adjust(r, g, b float64, o options) color.RGBA {
	l := 0.299*r + 0.587*g + 0.114*b
	f := func(v float64) uint8 {
		v = l + (v-l)*o.Saturation
		v = math.Max(0, math.Min(255, v))
		return uint8(math.Round(255 * math.Pow(v/255, o.Gamma)))
	}
	return color.RGBA{f(r), f(g), f(b), 255}
}

// ledPreview draws the panel as its LEDs will look (16 levels a channel
// after the driver's gamma curve), each LED a 7×7 block with a dark gap.
func ledPreview(c *matrix.Canvas) image.Image {
	const scale = 8
	img := image.NewRGBA(image.Rect(0, 0, matrix.Width*scale, matrix.Height*scale))
	for y := 0; y < matrix.Height*scale; y++ {
		for x := 0; x < matrix.Width*scale; x++ {
			col := color.RGBA{0, 0, 0, 255}
			if x%scale != scale-1 && y%scale != scale-1 {
				q := matrix.Quantize(c.Pix[y/scale][x/scale])
				col = color.RGBA{uint8(q>>8&15) * 17, uint8(q>>4&15) * 17, uint8(q&15) * 17, 255}
			}
			img.Set(x, y, col)
		}
	}
	return img
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func post(url, contentType string, body io.Reader) error {
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Post(url, contentType, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return fmt.Errorf("%s: %s %s", url, resp.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}
