package main

import (
	"encoding/json"
	"net/http"

	"dotmatrix/matrix"
)

// registerVideo serves POST /api/video, which plays a video on the panel.
func registerVideo(mux *http.ServeMux, player *matrix.Player, persist func()) {
	mux.HandleFunc("POST /api/video", func(w http.ResponseWriter, r *http.Request) {
		var req videoRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
			http.Error(w, `expected {"url":"https://www.youtube.com/watch?v=…","fit":false,"loop":false}`, http.StatusBadRequest)
			return
		}
		s, err := req.scene()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		player.Show(s)
		persist()
		writeState(w, player)
	})
}
