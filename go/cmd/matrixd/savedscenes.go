package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"dotmatrix/connect4"
	"dotmatrix/matrix"
)

const maxSceneName = 40

// handleScenes serves the saved scenes: what the panel shows, saved under a
// name to show again later.
//
//	GET    /api/scenes              [{"name":"Welcome","mode":"text","text":{...}}, ...]
//	POST   /api/scenes              {"name":"Welcome"} saves what the panel shows now
//	POST   /api/scenes/{name}/show  shows a saved scene (brightness is unchanged)
//	DELETE /api/scenes/{name}
func handleScenes(mux *http.ServeMux, player *matrix.Player, state *stateFile, attach func(*connect4.Game), persist func()) {
	list := func(w http.ResponseWriter) {
		scenes := state.sceneList()
		for i := range scenes {
			scenes[i].Image = nil // large and not useful to API clients
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(scenes)
	}
	mux.HandleFunc("GET /api/scenes", func(w http.ResponseWriter, r *http.Request) {
		list(w)
	})
	mux.HandleFunc("POST /api/scenes", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
			http.Error(w, `expected {"name": "..."}`, http.StatusBadRequest)
			return
		}
		name, err := sceneName(req.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cur := snapshot(player)
		if cur.Mode == "connect4" {
			http.Error(w, "a Connect 4 game can't be saved as a scene", http.StatusConflict)
			return
		}
		if err := state.putScene(savedScene{Name: name, content: cur}, cur); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		list(w)
	})
	mux.HandleFunc("POST /api/scenes/{name}/show", func(w http.ResponseWriter, r *http.Request) {
		s, ok := state.scene(r.PathValue("name"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		c := s.content
		c.Brightness = player.State().Brightness
		if err := c.apply(player, attach); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		persist()
		writeState(w, player)
	})
	mux.HandleFunc("DELETE /api/scenes/{name}", func(w http.ResponseWriter, r *http.Request) {
		found, err := state.deleteScene(r.PathValue("name"), snapshot(player))
		switch {
		case err != nil:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		case !found:
			http.NotFound(w, r)
			return
		}
		list(w)
	})
}

// sceneName checks and tidies a scene's name.
func sceneName(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", errors.New("the scene needs a name")
	case utf8.RuneCountInString(name) > maxSceneName:
		return "", errors.New("scene names can be at most 40 characters")
	case strings.ContainsFunc(name, func(r rune) bool { return unicode.IsControl(r) || r == '/' }):
		return "", errors.New(`scene names can't contain "/" or control characters`)
	}
	return name, nil
}
