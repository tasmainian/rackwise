package api

import (
	"errors"
	"net/http"

	"rackwise/engine/internal/bot"
	"rackwise/engine/internal/game"
	"rackwise/engine/internal/session"
)

type rulesResponse struct {
	BoardSize         int            `json:"board_size"`
	RackSize          int            `json:"rack_size"`
	Center            int            `json:"center"`
	BingoBonus        int            `json:"bingo_bonus"`
	MinBagForExchange int            `json:"min_bag_for_exchange"`
	Premiums          []string       `json:"premiums"`
	TileValues        map[string]int `json:"tile_values"`
	Levels            []bot.Level    `json:"levels"`
}

// rules gives the frontend the layout and tile values, so that they are defined
// only once, in the engine.
func (s *server) rules(w http.ResponseWriter, _ *http.Request) {
	premiums := make([]string, game.BoardSize)
	for r := range game.BoardSize {
		row := make([]byte, game.BoardSize)
		for c := range game.BoardSize {
			row[c] = byte(game.PremiumAt(r, c))
		}
		premiums[r] = string(row)
	}
	values := map[string]int{}
	for letter := byte('A'); letter <= 'Z'; letter++ {
		values[string(letter)] = game.TileValue(letter)
	}
	writeJSON(w, http.StatusOK, rulesResponse{
		BoardSize:         game.BoardSize,
		RackSize:          game.RackSize,
		Center:            game.Center,
		BingoBonus:        game.BingoBonus,
		MinBagForExchange: game.MinBagForExchange,
		Premiums:          premiums,
		TileValues:        values,
		Levels:            []bot.Level{bot.Easy, bot.Medium, bot.Hard},
	})
}

type createGameRequest struct {
	Level string  `json:"level"`
	Seed  *uint64 `json:"seed"`
}

func (s *server) createGame(w http.ResponseWriter, r *http.Request) {
	var req createGameRequest
	if !readJSON(w, r, &req) {
		return
	}
	level, err := bot.ParseLevel(req.Level)
	if err != nil {
		writeError(w, err)
		return
	}
	view, err := s.games.Create(r.Context(), level, req.Seed)
	writeGame(w, http.StatusCreated, view, err)
}

func (s *server) getGame(w http.ResponseWriter, r *http.Request) {
	view, err := s.games.Get(r.Context(), r.PathValue("id"))
	writeGame(w, http.StatusOK, view, err)
}

type playRequest struct {
	Tiles []tileJSON `json:"tiles"`
}

func (s *server) playGame(w http.ResponseWriter, r *http.Request) {
	var req playRequest
	if !readJSON(w, r, &req) {
		return
	}
	tiles := make([]game.Placement, len(req.Tiles))
	for i, t := range req.Tiles {
		if len(t.Letter) != 1 {
			writeError(w, errors.New("each tile needs a one-character letter"))
			return
		}
		tiles[i] = game.Placement{Row: t.Row, Col: t.Col, Letter: t.Letter[0]}
	}
	view, err := s.games.Play(r.Context(), r.PathValue("id"), tiles)
	writeGame(w, http.StatusOK, view, err)
}

type exchangeRequest struct {
	Tiles string `json:"tiles"`
}

func (s *server) exchangeGame(w http.ResponseWriter, r *http.Request) {
	var req exchangeRequest
	if !readJSON(w, r, &req) {
		return
	}
	view, err := s.games.Exchange(r.Context(), r.PathValue("id"), req.Tiles)
	writeGame(w, http.StatusOK, view, err)
}

func (s *server) passGame(w http.ResponseWriter, r *http.Request) {
	view, err := s.games.Pass(r.Context(), r.PathValue("id"))
	writeGame(w, http.StatusOK, view, err)
}

func (s *server) gameRecord(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	record, err := s.games.GCG(r.Context(), id)
	if err != nil {
		writeGameError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="rackwise-`+id+`.gcg"`)
	if _, err := w.Write([]byte(record)); err != nil {
		internalError(w, err)
	}
}

func writeGame(w http.ResponseWriter, status int, view *session.View, err error) {
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, status, view)
}

// writeGameError maps game errors to statuses. A move that breaks the rules is
// 422 with a code the frontend can act on, unlike /v1/validate, where asking about
// an illegal play is the normal use.
func writeGameError(w http.ResponseWriter, err error) {
	var rejection *session.Rejection
	switch {
	case errors.As(err, &rejection):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"code": rejection.Code, "error": rejection.Message})
	case errors.Is(err, session.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"code": "not_found", "error": err.Error()})
	case errors.Is(err, session.ErrGameOver):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "game_over", "error": err.Error()})
	case errors.Is(err, session.ErrNotFinished):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "not_finished", "error": err.Error()})
	case errors.Is(err, session.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "conflict", "error": err.Error()})
	default:
		internalError(w, err)
	}
}
