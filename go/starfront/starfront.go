// Package starfront reads the roof state of the Starfront Observatory
// buildings from its Alpaca API and shows it as a matrix.Scene:
//
//	  STARFRONT
//	🔭 CLOSED       (or OPEN, or MIXED)
//	ALL 14 CLOSED
//	■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■ ■   one square per building
package starfront

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"
)

// DefaultURL is the roof state endpoint.
const DefaultURL = "https://alpaca-api.tx.starfront.space/api/v1/roof/state"

// Roof is one building's roll-off roof.
type Roof struct {
	Device  int       `json:"device_number"`
	Name    string    `json:"name"` // "Building 1"
	Open    bool      `json:"is_open"`
	Updated time.Time `json:"state_update"` // when the observatory last read it
}

// StaleAfter is how old a reading can be before the scene treats that
// building's state as unknown.
const StaleAfter = 30 * time.Minute

// Stale reports whether the reading is too old to trust at time now.
func (r Roof) Stale(now time.Time) bool { return now.Sub(r.Updated) > StaleAfter }

var client = &http.Client{Timeout: 10 * time.Second}

// Fetch returns every building's roof, sorted by device number.
func Fetch(ctx context.Context, url string) ([]Roof, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("starfront: %s", resp.Status)
	}
	var roofs []Roof
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&roofs); err != nil {
		return nil, fmt.Errorf("starfront: %w", err)
	}
	sort.Slice(roofs, func(i, j int) bool { return roofs[i].Device < roofs[j].Device })
	return roofs, nil
}
