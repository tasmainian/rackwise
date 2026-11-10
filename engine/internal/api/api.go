// Package api is the engine's HTTP interface. See docs/engine-api.md.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"net/http"

	"rackwise/engine/internal/bot"
	"rackwise/engine/internal/eval"
	"rackwise/engine/internal/game"
	"rackwise/engine/internal/movegen"
	"rackwise/engine/internal/session"
)

const maxRequestBytes = 16 << 10

// A full bag minus the two opening racks is the most a request can claim.
const maxBag = 100 - 2*game.RackSize

// bagUnknown stands in when a client doesn't say how many tiles remain. Any
// value above zero treats the game as still having tiles to draw.
const bagUnknown = maxBag

// New returns the engine's routes.
func New(bots *bot.Bots, games *session.Service) http.Handler {
	s := &server{bots: bots, games: games}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/rules", s.rules)
	mux.HandleFunc("POST /v1/games", s.createGame)
	mux.HandleFunc("GET /v1/games/{id}", s.getGame)
	mux.HandleFunc("POST /v1/games/{id}/play", s.playGame)
	mux.HandleFunc("POST /v1/games/{id}/exchange", s.exchangeGame)
	mux.HandleFunc("POST /v1/games/{id}/pass", s.passGame)
	mux.HandleFunc("GET /v1/games/{id}/gcg", s.gameRecord)
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /v1/moves", s.moves)
	mux.HandleFunc("POST /v1/validate", s.validate)
	mux.HandleFunc("POST /v1/bot-move", s.botMove)
	return mux
}

type server struct {
	bots  *bot.Bots
	games *session.Service
}

type tileJSON struct {
	Row    int    `json:"row"`
	Col    int    `json:"col"`
	Letter string `json:"letter"`
}

type moveJSON struct {
	Row         int        `json:"row"`
	Col         int        `json:"col"`
	Direction   string     `json:"direction"`
	Position    string     `json:"position"`
	Word        string     `json:"word"`
	Tiles       []tileJSON `json:"tiles"`
	CrossWords  []string   `json:"cross_words"`
	Score       int        `json:"score"`
	Bingo       bool       `json:"bingo"`
	Leave       string     `json:"leave"`
	LeaveValue  float64    `json:"leave_value"`
	LanesOpened int        `json:"lanes_opened"`
	Equity      float64    `json:"equity"`
}

func newMoveJSON(move *game.Move, evaluation eval.Evaluation) moveJSON {
	tiles := make([]tileJSON, len(move.Tiles))
	for i, t := range move.Tiles {
		tiles[i] = tileJSON{Row: t.Row, Col: t.Col, Letter: string(t.Letter)}
	}
	crossWords := move.CrossWords
	if crossWords == nil {
		crossWords = []string{}
	}
	return moveJSON{
		Row:         move.Row,
		Col:         move.Col,
		Direction:   move.Dir.String(),
		Position:    move.Position(),
		Word:        move.Word,
		Tiles:       tiles,
		CrossWords:  crossWords,
		Score:       move.Score,
		Bingo:       move.Bingo,
		Leave:       evaluation.Leave,
		LeaveValue:  roundPoints(evaluation.LeaveValue),
		LanesOpened: evaluation.LanesOpened,
		Equity:      roundPoints(evaluation.Equity),
	}
}

// roundPoints keeps values at the precision the leave table has, so clients don't
// see floating-point noise like 1.4300000000000002.
func roundPoints(value float64) float64 {
	return math.Round(value*100) / 100
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "engine"})
}

type movesRequest struct {
	Board        []string `json:"board"`
	Rack         string   `json:"rack"`
	Limit        int      `json:"limit"`
	BagRemaining *int     `json:"bag_remaining"`
}

type movesResponse struct {
	// Total counts every legal move, even when Limit shortens the list.
	Total int        `json:"total"`
	Moves []moveJSON `json:"moves"`
}

func (s *server) moves(w http.ResponseWriter, r *http.Request) {
	var req movesRequest
	if !readJSON(w, r, &req) {
		return
	}
	board, rack, err := parsePosition(req.Board, req.Rack)
	if err != nil {
		writeError(w, err)
		return
	}
	if req.Limit < 0 {
		writeError(w, errors.New("limit must not be negative"))
		return
	}
	bag := bagUnknown
	if req.BagRemaining != nil {
		if bag, err = parseBag(*req.BagRemaining); err != nil {
			writeError(w, err)
			return
		}
	}

	moves := movegen.Generate(board, rack, s.bots.Lex)
	total := len(moves)
	if req.Limit > 0 && req.Limit < total {
		moves = moves[:req.Limit]
	}
	response := movesResponse{Total: total, Moves: make([]moveJSON, len(moves))}
	for i, move := range moves {
		evaluation, err := s.bots.Eval.Evaluate(board, move, rack, bag)
		if err != nil {
			internalError(w, err)
			return
		}
		response.Moves[i] = newMoveJSON(move, evaluation)
	}
	writeJSON(w, http.StatusOK, response)
}

type validateRequest struct {
	Board        []string   `json:"board"`
	Rack         string     `json:"rack"`
	Tiles        []tileJSON `json:"tiles"`
	BagRemaining *int       `json:"bag_remaining"`
}

// validateResponse describes a well-formed request. An illegal play is a normal
// answer rather than a client error, so it still returns 200.
type validateResponse struct {
	Legal  bool      `json:"legal"`
	Code   string    `json:"code,omitempty"`
	Reason string    `json:"reason,omitempty"`
	Move   *moveJSON `json:"move,omitempty"`
}

func (s *server) validate(w http.ResponseWriter, r *http.Request) {
	var req validateRequest
	if !readJSON(w, r, &req) {
		return
	}
	board, rack, err := parsePosition(req.Board, req.Rack)
	if err != nil {
		writeError(w, err)
		return
	}
	bag := bagUnknown
	if req.BagRemaining != nil {
		if bag, err = parseBag(*req.BagRemaining); err != nil {
			writeError(w, err)
			return
		}
	}
	tiles := make([]game.Placement, len(req.Tiles))
	for i, t := range req.Tiles {
		if len(t.Letter) != 1 {
			writeError(w, fmt.Errorf("tile %d: letter must be one character", i))
			return
		}
		tiles[i] = game.Placement{Row: t.Row, Col: t.Col, Letter: t.Letter[0]}
	}

	move, err := game.Validate(board, rack, tiles, s.bots.Lex)
	if err != nil {
		code := game.CodeOf(err)
		if code == "" {
			internalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, validateResponse{Code: string(code), Reason: err.Error()})
		return
	}
	evaluation, err := s.bots.Eval.Evaluate(board, move, rack, bag)
	if err != nil {
		internalError(w, err)
		return
	}
	result := newMoveJSON(move, evaluation)
	writeJSON(w, http.StatusOK, validateResponse{Legal: true, Move: &result})
}

type botMoveRequest struct {
	Board        []string `json:"board"`
	Rack         string   `json:"rack"`
	Level        string   `json:"level"`
	BagRemaining *int     `json:"bag_remaining"`
	Seed         *uint64  `json:"seed"`
}

type botMoveResponse struct {
	Kind     string    `json:"kind"`
	Move     *moveJSON `json:"move,omitempty"`
	Exchange string    `json:"exchange,omitempty"`
	// Seed is returned so that any choice can be replayed.
	Seed uint64 `json:"seed"`
}

func (s *server) botMove(w http.ResponseWriter, r *http.Request) {
	var req botMoveRequest
	if !readJSON(w, r, &req) {
		return
	}
	board, rack, err := parsePosition(req.Board, req.Rack)
	if err != nil {
		writeError(w, err)
		return
	}
	level, err := bot.ParseLevel(req.Level)
	if err != nil {
		writeError(w, err)
		return
	}
	// Required here, unlike the other endpoints: whether a bot may exchange, and
	// how it values a leave, both depend on it.
	if req.BagRemaining == nil {
		writeError(w, errors.New("bag_remaining is required"))
		return
	}
	bag, err := parseBag(*req.BagRemaining)
	if err != nil {
		writeError(w, err)
		return
	}
	seed := rand.Uint64()
	if req.Seed != nil {
		seed = *req.Seed
	}

	action, err := s.bots.Choose(level, board, rack, bag, rand.New(rand.NewPCG(seed, 0)))
	if err != nil {
		internalError(w, err)
		return
	}
	response := botMoveResponse{Kind: string(action.Kind), Exchange: action.Exchange, Seed: seed}
	if action.Kind == game.TurnPlay {
		result := newMoveJSON(action.Move, action.Evaluation)
		response.Move = &result
	}
	writeJSON(w, http.StatusOK, response)
}

func parsePosition(rows []string, rack string) (*game.Board, string, error) {
	board, err := game.ParseBoard(rows)
	if err != nil {
		return nil, "", err
	}
	parsed, err := game.ParseRack(rack)
	if err != nil {
		return nil, "", err
	}
	return board, parsed, nil
}

func parseBag(bag int) (int, error) {
	if bag < 0 || bag > maxBag {
		return 0, fmt.Errorf("bag_remaining must be between 0 and %d", maxBag)
	}
	return bag, nil
}

func readJSON(w http.ResponseWriter, r *http.Request, into any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		writeError(w, fmt.Errorf("invalid request: %w", err))
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}

func internalError(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("writing response: %v", err)
	}
}
