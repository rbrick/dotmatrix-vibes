package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"dotmatrix/connect4"
	"dotmatrix/matrix"
)

func handleConnect4(mux *http.ServeMux, player *matrix.Player, attach func(*connect4.Game), persist func()) {
	current := func() *connect4.Game {
		g, _ := player.State().Scene.(*connect4.Game)
		return g
	}
	reply := func(w http.ResponseWriter, g *connect4.Game) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(g.State())
	}
	// withGame runs f on the game being shown, then redraws and saves.
	withGame := func(f func(http.ResponseWriter, *http.Request, *connect4.Game) error) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			g := current()
			if g == nil {
				http.Error(w, "no game in progress; POST /api/connect4/new first", http.StatusConflict)
				return
			}
			if err := f(w, r, g); err != nil {
				status := http.StatusBadRequest
				if errors.Is(err, connect4.ErrGameOver) || errors.Is(err, connect4.ErrNotYourTurn) ||
					errors.Is(err, connect4.ErrColumnFull) {
					status = http.StatusConflict
				}
				http.Error(w, err.Error(), status)
				return
			}
			player.Redraw()
			persist()
			reply(w, g)
		}
	}

	mux.HandleFunc("GET /api/connect4", func(w http.ResponseWriter, r *http.Request) {
		g := current()
		if g == nil {
			http.Error(w, "no game in progress", http.StatusNotFound)
			return
		}
		reply(w, g)
	})

	mux.HandleFunc("POST /api/connect4/new", func(w http.ResponseWriter, r *http.Request) {
		var opts connect4.Options
		var req struct {
			VsComputer *bool               `json:"vsComputer"`
			Difficulty connect4.Difficulty `json:"difficulty"`
			CPUFirst   bool                `json:"cpuFirst"`
		}
		if r.ContentLength != 0 {
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
				http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
				return
			}
		}
		switch req.Difficulty {
		case "", connect4.Easy, connect4.Normal, connect4.Hard:
		default:
			http.Error(w, `difficulty must be "easy", "normal" or "hard"`, http.StatusBadRequest)
			return
		}
		opts = connect4.Options{VsComputer: req.VsComputer == nil || *req.VsComputer, Difficulty: req.Difficulty, CPUFirst: req.CPUFirst}

		// Another round against the same kind of opponent keeps the score.
		g := current()
		if g != nil && g.State().VsComputer == opts.VsComputer {
			g.Reset(opts)
		} else {
			g = connect4.New(connect4.Options{})
			attach(g)
			g.Reset(opts) // after attach, so a computer first move is shown
		}
		player.Show(g)
		persist()
		reply(w, g)
	})

	mux.HandleFunc("POST /api/connect4/move", withGame(func(w http.ResponseWriter, r *http.Request, g *connect4.Game) error {
		var req struct {
			Column *int `json:"column"`
		}
		if r.ContentLength != 0 {
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
				return err
			}
		}
		if req.Column == nil {
			return g.DropAtCursor()
		}
		g.SetCursor(*req.Column)
		return g.Drop(*req.Column)
	}))

	mux.HandleFunc("POST /api/connect4/cursor", withGame(func(w http.ResponseWriter, r *http.Request, g *connect4.Game) error {
		var req struct {
			Column *int `json:"column"`
			Delta  int  `json:"delta"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
			return err
		}
		if req.Column != nil {
			return g.SetCursor(*req.Column)
		}
		g.MoveCursor(req.Delta)
		return nil
	}))
}
