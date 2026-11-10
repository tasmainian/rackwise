package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rackwise/engine/internal/bot"
	"rackwise/engine/internal/game"
	"rackwise/engine/internal/session"
)

// sharedHandler keeps games alive between requests within a test.
var sharedHandler http.Handler

func handler(t *testing.T) http.Handler {
	t.Helper()
	if sharedHandler != nil {
		return sharedHandler
	}
	bots, err := bot.Default()
	if err != nil {
		t.Fatal(err)
	}
	sharedHandler = New(bots, session.New(bots, session.NewMemoryStore()))
	return sharedHandler
}

func post(t *testing.T, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var text string
	switch b := body.(type) {
	case string:
		text = b
	default:
		encoded, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		text = string(encoded)
	}
	recorder := httptest.NewRecorder()
	handler(t).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader(text)))
	return recorder
}

func decode[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var value T
	if err := json.Unmarshal(recorder.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func emptyBoard() []string { return (&game.Board{}).Rows() }

func TestHealth(t *testing.T) {
	recorder := httptest.NewRecorder()
	handler(t).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"ok"`) {
		t.Errorf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
}

func TestMovesReturnsRankedMovesWithLeaves(t *testing.T) {
	got := decode[movesResponse](t, post(t, "/v1/moves", movesRequest{Board: emptyBoard(), Rack: "AEINRST"}))
	if got.Total == 0 || got.Total != len(got.Moves) {
		t.Fatalf("total = %d, moves = %d", got.Total, len(got.Moves))
	}
	best := got.Moves[0]
	if !best.Bingo || best.Score != 56 || best.Leave != "" {
		t.Errorf("best move = %+v", best)
	}
	for i := 1; i < len(got.Moves); i++ {
		if got.Moves[i].Score > got.Moves[i-1].Score {
			t.Fatalf("moves not ranked at %d", i)
		}
	}
	for _, move := range got.Moves {
		if len(move.Leave)+len(move.Tiles) != 7 {
			t.Fatalf("%s: leave %q and %d tiles don't add up to the rack", move.Word, move.Leave, len(move.Tiles))
		}
		if move.CrossWords == nil {
			t.Fatal("cross_words should be an empty list, not null")
		}
	}
}

func TestMovesLimitKeepsTotal(t *testing.T) {
	all := decode[movesResponse](t, post(t, "/v1/moves", movesRequest{Board: emptyBoard(), Rack: "AEINRST"}))
	limited := decode[movesResponse](t, post(t, "/v1/moves", movesRequest{Board: emptyBoard(), Rack: "AEINRST", Limit: 5}))
	if len(limited.Moves) != 5 || limited.Total != all.Total {
		t.Errorf("moves = %d, total = %d, want 5 and %d", len(limited.Moves), limited.Total, all.Total)
	}
}

func TestMovesWithNoPlaysReturnsEmptyList(t *testing.T) {
	recorder := post(t, "/v1/moves", movesRequest{Board: emptyBoard(), Rack: "QQ"})
	if !strings.Contains(recorder.Body.String(), `"moves":[]`) {
		t.Errorf("body = %s", recorder.Body)
	}
}

func TestMovesUsesBagForLeaveValue(t *testing.T) {
	zero := 0
	got := decode[movesResponse](t, post(t, "/v1/moves", movesRequest{Board: emptyBoard(), Rack: "ATQ", BagRemaining: &zero}))
	for _, move := range got.Moves {
		if move.Word == "AT" && move.LeaveValue != -10 {
			t.Errorf("leave value = %v, want -10 for a Q left with an empty bag", move.LeaveValue)
		}
	}
}

func TestPointsAreRounded(t *testing.T) {
	got := decode[movesResponse](t, post(t, "/v1/moves", movesRequest{Board: emptyBoard(), Rack: "AEINRS?"}))
	for _, move := range got.Moves {
		if move.LeaveValue != roundPoints(move.LeaveValue) || move.Equity != roundPoints(move.Equity) {
			t.Fatalf("%s: leave value %v, equity %v", move.Word, move.LeaveValue, move.Equity)
		}
	}
}

func TestBadRequests(t *testing.T) {
	negative := -1
	tooMany := 500
	cases := []struct {
		name string
		path string
		body any
	}{
		{"not json", "/v1/moves", "{"},
		{"unknown field", "/v1/moves", `{"board":[],"rack":"A","surprise":1}`},
		{"short board", "/v1/moves", movesRequest{Board: []string{"..."}, Rack: "A"}},
		{"bad rack character", "/v1/moves", movesRequest{Board: emptyBoard(), Rack: "a1"}},
		{"rack too long", "/v1/moves", movesRequest{Board: emptyBoard(), Rack: "ABCDEFGH"}},
		{"negative limit", "/v1/moves", movesRequest{Board: emptyBoard(), Rack: "A", Limit: -1}},
		{"negative bag", "/v1/moves", movesRequest{Board: emptyBoard(), Rack: "A", BagRemaining: &negative}},
		{"oversized bag", "/v1/moves", movesRequest{Board: emptyBoard(), Rack: "A", BagRemaining: &tooMany}},
		{"body too large", "/v1/moves", `{"rack":"` + strings.Repeat("A", maxRequestBytes) + `"}`},
		{"multi-character tile", "/v1/validate", validateRequest{Board: emptyBoard(), Rack: "A", Tiles: []tileJSON{{7, 7, "AB"}}}},
		{"unknown level", "/v1/bot-move", botMoveRequest{Board: emptyBoard(), Rack: "A", Level: "expert"}},
		{"bot without bag", "/v1/bot-move", botMoveRequest{Board: emptyBoard(), Rack: "A", Level: "easy"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := post(t, tc.path, tc.body)
			if recorder.Code != http.StatusBadRequest {
				t.Errorf("status = %d, body = %s", recorder.Code, recorder.Body)
			}
			if !strings.Contains(recorder.Body.String(), `"error"`) {
				t.Errorf("body should explain the error: %s", recorder.Body)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	legal := decode[validateResponse](t, post(t, "/v1/validate", validateRequest{
		Board: emptyBoard(),
		Rack:  "ATS",
		Tiles: []tileJSON{{7, 7, "A"}, {7, 8, "T"}},
	}))
	if !legal.Legal || legal.Move == nil || legal.Move.Score != 2 || legal.Move.Leave != "S" {
		t.Errorf("legal play: %+v", legal)
	}

	// Q is 10 and I is 1, with no premium on either square.
	qi := decode[validateResponse](t, post(t, "/v1/validate", validateRequest{
		Board: emptyBoard(),
		Rack:  "QI",
		Tiles: []tileJSON{{7, 7, "Q"}, {7, 8, "I"}},
	}))
	if !qi.Legal || qi.Move == nil || qi.Move.Word != "QI" || qi.Move.Score != 11 {
		t.Errorf("QI should score 11: %+v", qi)
	}

	illegal := decode[validateResponse](t, post(t, "/v1/validate", validateRequest{
		Board: emptyBoard(),
		Rack:  "ZA",
		Tiles: []tileJSON{{7, 7, "Z"}, {7, 8, "A"}},
	}))
	if illegal.Legal || illegal.Code != string(game.ErrInvalidWord) || illegal.Move != nil {
		t.Errorf("ZA should be an invalid word: %+v", illegal)
	}
}

func TestBotMove(t *testing.T) {
	bag := 50
	seed := uint64(7)
	for _, level := range []string{"easy", "medium", "hard"} {
		req := botMoveRequest{Board: emptyBoard(), Rack: "AEINRST", Level: level, BagRemaining: &bag, Seed: &seed}
		got := decode[botMoveResponse](t, post(t, "/v1/bot-move", req))
		if got.Kind != "play" || got.Move == nil || got.Seed != 7 {
			t.Errorf("%s: %+v", level, got)
		}
		again := decode[botMoveResponse](t, post(t, "/v1/bot-move", req))
		if again.Move.Position != got.Move.Position || again.Move.Word != got.Move.Word {
			t.Errorf("%s: the same seed gave a different move", level)
		}
	}
}

func TestBotMoveExchangesWithNoPlay(t *testing.T) {
	bag := 50
	got := decode[botMoveResponse](t, post(t, "/v1/bot-move", botMoveRequest{
		Board: emptyBoard(), Rack: "QQ", Level: "medium", BagRemaining: &bag,
	}))
	if got.Kind != "exchange" || got.Exchange != "QQ" || got.Move != nil {
		t.Errorf("%+v", got)
	}
}

func TestRules(t *testing.T) {
	recorder := httptest.NewRecorder()
	handler(t).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/rules", nil))
	got := decode[rulesResponse](t, recorder)
	if got.BoardSize != 15 || len(got.Premiums) != 15 || got.Premiums[0][3] != 'T' || got.Premiums[3][7] != 'D' {
		t.Errorf("rules = %+v", got)
	}
	if got.TileValues["Q"] != 10 || got.TileValues["E"] != 1 || len(got.TileValues) != 26 {
		t.Errorf("values = %v", got.TileValues)
	}
}

func get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler(t).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

func TestGameFlow(t *testing.T) {
	seed := uint64(11)
	created := post(t, "/v1/games", createGameRequest{Level: "medium", Seed: &seed})
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body)
	}
	var view session.View
	if err := json.Unmarshal(created.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	base := "/v1/games/" + view.ID

	// An illegal play is 422 with a reason, and doesn't use up the turn.
	illegal := post(t, base+"/play", playRequest{Tiles: []tileJSON{{0, 0, string(view.Rack[0])}}})
	if illegal.Code != http.StatusUnprocessableEntity || !strings.Contains(illegal.Body.String(), `"code"`) {
		t.Errorf("illegal play: %d %s", illegal.Code, illegal.Body)
	}

	if got := get(t, base+"/gcg"); got.Code != http.StatusConflict {
		t.Errorf("record of an active game: %d", got.Code)
	}

	// Pass until six scoreless turns end the game, if the bot also has nothing better.
	exchanged := decode[session.View](t, post(t, base+"/exchange", exchangeRequest{Tiles: view.Rack[:1]}))
	if len(exchanged.Turns) != 2 {
		t.Errorf("turns = %+v", exchanged.Turns)
	}
	passed := decode[session.View](t, post(t, base+"/pass", "{}"))
	if len(passed.Turns) != 4 {
		t.Errorf("turns = %+v", passed.Turns)
	}
	loaded := decode[session.View](t, get(t, base))
	if len(loaded.Turns) != 4 {
		t.Errorf("loaded turns = %+v", loaded.Turns)
	}
}

func TestFinishedGameRecordDownload(t *testing.T) {
	seed := uint64(12)
	created := post(t, "/v1/games", createGameRequest{Level: "easy", Seed: &seed})
	var view session.View
	if err := json.Unmarshal(created.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	base := "/v1/games/" + view.ID
	// Always passing still ends the game, because the bot eventually can't score.
	for range 200 {
		recorder := post(t, base+"/pass", "{}")
		if recorder.Code == http.StatusConflict {
			break
		}
		if recorder.Code != http.StatusOK {
			t.Fatalf("pass: %d %s", recorder.Code, recorder.Body)
		}
	}
	record := get(t, base+"/gcg")
	if record.Code != http.StatusOK || !strings.HasPrefix(record.Body.String(), "#character-encoding") {
		t.Fatalf("record: %d %s", record.Code, record.Body)
	}
	if !strings.Contains(record.Header().Get("Content-Disposition"), ".gcg") {
		t.Errorf("headers = %v", record.Header())
	}
	if got := post(t, base+"/pass", "{}"); got.Code != http.StatusConflict {
		t.Errorf("pass after the end: %d", got.Code)
	}
}

func TestGameErrors(t *testing.T) {
	if got := get(t, "/v1/games/nope"); got.Code != http.StatusNotFound {
		t.Errorf("missing game: %d", got.Code)
	}
	if got := post(t, "/v1/games", createGameRequest{Level: "expert"}); got.Code != http.StatusBadRequest {
		t.Errorf("bad level: %d", got.Code)
	}
	if got := post(t, "/v1/games/nope/play", playRequest{Tiles: []tileJSON{{0, 0, "AB"}}}); got.Code != http.StatusBadRequest {
		t.Errorf("bad letter: %d", got.Code)
	}
}
