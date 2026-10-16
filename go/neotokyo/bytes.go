// Package neotokyo shows things from the Neo Tokyo NFT project: the price of
// its $BYTES token, and Citizens rendered by a citgen server.
package neotokyo

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"dotmatrix/matrix"
)

// DefaultPriceURL is the $BYTES price endpoint.
const DefaultPriceURL = "https://api.bytes.onl/v1/api/price"

// Price is a $BYTES quote.
type Price struct {
	USD       float64
	ETH       float64
	Supply    float64 // tokens in circulation
	MarketCap float64 // in USD
}

var client = &http.Client{Timeout: 30 * time.Second}

// FetchPrice reads the current $BYTES price. The API sends numbers as
// strings: {"prices":{"usdPrice":"0.29","etherPrice":"0.00012"},
// "totalSupply":"5253722.7","marketCap":"1561206.7"}.
func FetchPrice(ctx context.Context, url string) (Price, error) {
	var raw struct {
		Prices struct {
			USD json.Number `json:"usdPrice"`
			ETH json.Number `json:"etherPrice"`
		} `json:"prices"`
		Supply    json.Number `json:"totalSupply"`
		MarketCap json.Number `json:"marketCap"`
	}
	body, err := get(ctx, url, "application/json")
	if err != nil {
		return Price{}, fmt.Errorf("bytes price: %w", err)
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Price{}, fmt.Errorf("bytes price: %w", err)
	}
	var p Price
	for _, f := range []struct {
		n   json.Number
		v   *float64
		req bool
	}{{raw.Prices.USD, &p.USD, true}, {raw.Prices.ETH, &p.ETH, false}, {raw.Supply, &p.Supply, false}, {raw.MarketCap, &p.MarketCap, false}} {
		if f.n == "" && !f.req {
			continue
		}
		if *f.v, err = strconv.ParseFloat(string(f.n), 64); err != nil {
			return Price{}, fmt.Errorf("bytes price: bad number %q", f.n)
		}
	}
	return p, nil
}

// statusError is a non-200 response; Body is the server's message.
type statusError struct {
	Code int
	Body string
}

func (e *statusError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("%d %s: %s", e.Code, http.StatusText(e.Code), e.Body)
	}
	return fmt.Sprintf("%d %s", e.Code, http.StatusText(e.Code))
}

func get(ctx context.Context, url, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, &statusError{resp.StatusCode, msg}
	}
	return body, nil
}

// PriceOptions configures a PriceScene.
type PriceOptions struct {
	Interval time.Duration // how often to check while shown; default 30s, at least MinPriceInterval
	URL      string        // default DefaultPriceURL
}

// Price check intervals.
const (
	DefaultPriceInterval = 30 * time.Second
	MinPriceInterval     = 10 * time.Second
)

// PriceScene shows the $BYTES price:
//
//	   $BYTES
//	  $0.2972      green or red: up or down since the last change
//	0.000122 ETH
//	MCAP 1.56M
type PriceScene struct {
	PriceOptions

	mu       sync.Mutex
	price    *Price
	err      error
	checked  time.Time
	fetching bool
	trend    int // +1 or -1: the direction of the last change, 0 before one
	fetch    func(ctx context.Context, url string) (Price, error)
}

// NewPrice checks the options; it doesn't contact the API.
func NewPrice(opts PriceOptions) (*PriceScene, error) {
	switch {
	case opts.Interval == 0:
		opts.Interval = DefaultPriceInterval
	case opts.Interval < MinPriceInterval:
		return nil, fmt.Errorf("bytes price: interval must be at least %v", MinPriceInterval)
	}
	if opts.URL == "" {
		opts.URL = DefaultPriceURL
	}
	return &PriceScene{PriceOptions: opts, fetch: FetchPrice}, nil
}

// Refresh reads the price now and waits for the answer. After an error the
// last good price is kept.
func (s *PriceScene) Refresh(ctx context.Context) (Price, error) {
	p, err := s.fetch(ctx, s.URL)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err, s.checked = err, time.Now()
	if err == nil {
		if s.price != nil && p.USD != s.price.USD {
			s.trend = 1
			if p.USD < s.price.USD {
				s.trend = -1
			}
		}
		s.price = &p
	}
	return p, err
}

// Price returns the last good price (nil before the first) and the error
// from the latest check, if it failed.
func (s *PriceScene) Price() (*Price, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.price, s.err
}

func (s *PriceScene) refreshIfDue() {
	s.mu.Lock()
	due := !s.fetching && (s.checked.IsZero() || time.Since(s.checked) >= s.Interval)
	if due {
		s.fetching = true
	}
	s.mu.Unlock()
	if !due {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		s.Refresh(ctx)
		s.mu.Lock()
		s.fetching = false
		s.mu.Unlock()
	}()
}

var (
	pink      = color.RGBA{255, 40, 160, 255} // Neo Tokyo's neon
	cyan      = color.RGBA{0, 220, 255, 255}
	white     = color.RGBA{255, 255, 255, 255}
	grey      = color.RGBA{140, 140, 140, 255}
	upColor   = color.RGBA{60, 255, 60, 255}
	downColor = color.RGBA{255, 50, 40, 255}
)

// Render implements matrix.Scene.
func (s *PriceScene) Render(c *matrix.Canvas, _ time.Duration) time.Duration {
	s.refreshIfDue()
	s.mu.Lock()
	p, err, trend := s.price, s.err, s.trend
	s.mu.Unlock()

	c.Clear()
	center := func(y int, text string, col color.Color) {
		c.DrawText((matrix.Width-matrix.TextWidth(text))/2, y, text, col)
	}
	small := func(y int, text string, col color.Color) {
		c.DrawSmallText((matrix.Width-matrix.SmallTextWidth(text))/2, y, text, col)
	}
	center(0, "$BYTES", pink)
	switch {
	case p == nil && err != nil:
		center(10, "NO DATA", grey)
		small(21, "NO CONNECTION", grey)
	case p == nil:
		center(10, "...", grey)
	default:
		col := white
		switch trend {
		case 1:
			col = upColor
		case -1:
			col = downColor
		}
		center(10, "$"+usd(p.USD), col)
		if p.ETH > 0 {
			small(20, sig(p.ETH, 3)+" ETH", cyan)
		}
		if err != nil {
			small(26, "NO CONNECTION", grey)
		} else if p.MarketCap > 0 {
			small(26, "MCAP "+compact(p.MarketCap), grey) // a 3x5 "$" looks like a 3
		}
	}
	return time.Second / 2 // show new prices promptly
}

// usd formats a price to fit the big font: 4 significant digits below $1,
// cents up to $1000, then K and M.
func usd(v float64) string {
	switch {
	case v < 1:
		return sig(v, 4)
	case v < 1000:
		return strconv.FormatFloat(v, 'f', 2, 64)
	}
	return compact(v)
}

// sig formats v with n significant digits and no exponent.
func sig(v float64, n int) string {
	if v == 0 {
		return "0"
	}
	decimals := max(0, n-1-int(math.Floor(math.Log10(math.Abs(v)))))
	return strconv.FormatFloat(v, 'f', decimals, 64)
}

// compact formats v as 950, 12.3K, 1.56M or 2.1B.
func compact(v float64) string {
	for _, u := range []struct {
		div    float64
		suffix string
	}{{1e9, "B"}, {1e6, "M"}, {1e3, "K"}} {
		if v >= u.div {
			return sig(v/u.div, 3) + u.suffix
		}
	}
	return sig(v, 3)
}
