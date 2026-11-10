package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"rackwise/engine/internal/bot"
	"rackwise/engine/internal/game"
	"rackwise/engine/internal/movegen"
)

var ctx = context.Background()

func newService(t *testing.T) (*Service, *bot.Bots, Store) {
	t.Helper()
	bots, err := bot.Default()
	if err != nil {
		t.Fatal(err)
	}
	store := NewMemoryStore()
	return New(bots, store), bots, store
}

func seedPointer(seed uint64) *uint64 { return &seed }

// bestPlay finds the person's best move, going through the view like a client.
func bestPlay(t *testing.T, bots *bot.Bots, view *View) []game.Placement {
	t.Helper()
	board, err := game.ParseBoard(view.Board)
	if err != nil {
		t.Fatal(err)
	}
	moves := movegen.Generate(board, view.Rack, bots.Lex)
	if len(moves) == 0 {
		return nil
	}
	return moves[0].Tiles
}

// finishGame plays the person's side until the end.
func finishGame(t *testing.T, s *Service, bots *bot.Bots, view *View) *View {
	t.Helper()
	var err error
	for view.Status == "active" {
		tiles := bestPlay(t, bots, view)
		switch {
		case tiles != nil:
			view, err = s.Play(ctx, view.ID, tiles)
		case view.CanExchange:
			view, err = s.Exchange(ctx, view.ID, view.Rack)
		default:
			view, err = s.Pass(ctx, view.ID)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	return view
}

func TestNewGameView(t *testing.T) {
	s, _, _ := newService(t)
	view, err := s.Create(ctx, bot.Medium, seedPointer(1))
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != "active" || len(view.Rack) != game.RackSize || view.BotTiles != game.RackSize {
		t.Errorf("view = %+v", view)
	}
	if view.BagRemaining != 86 || !view.CanExchange || len(view.Board) != game.BoardSize {
		t.Errorf("view = %+v", view)
	}
	if view.Result != nil {
		t.Error("a new game has no result")
	}
}

func TestPlayGetsABotReply(t *testing.T) {
	s, bots, _ := newService(t)
	view, err := s.Create(ctx, bot.Medium, seedPointer(1))
	if err != nil {
		t.Fatal(err)
	}
	tiles := bestPlay(t, bots, view)
	view, err = s.Play(ctx, view.ID, tiles)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Turns) != 2 || view.Turns[0].Player != "you" || view.Turns[1].Player != "bot" {
		t.Fatalf("turns = %+v", view.Turns)
	}
	if view.Scores.You != view.Turns[0].Total || view.Scores.Bot != view.Turns[1].Total {
		t.Errorf("scores %+v don't match turns %+v", view.Scores, view.Turns)
	}
	if len(view.Rack) != game.RackSize {
		t.Errorf("rack = %q, should be refilled", view.Rack)
	}

	loaded, err := s.Get(ctx, view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(loaded.Board, "") != strings.Join(view.Board, "") || loaded.Scores != view.Scores {
		t.Error("the saved game differs from the returned one")
	}
}

func TestIllegalPlayIsRejectedAndChangesNothing(t *testing.T) {
	s, _, _ := newService(t)
	view, err := s.Create(ctx, bot.Easy, seedPointer(1))
	if err != nil {
		t.Fatal(err)
	}
	tiles := []game.Placement{{Row: 0, Col: 0, Letter: view.Rack[0]}}
	_, err = s.Play(ctx, view.ID, tiles)
	var rejection *Rejection
	if !errors.As(err, &rejection) || rejection.Code == "" {
		t.Fatalf("err = %v", err)
	}
	after, err := s.Get(ctx, view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Turns) != 0 || after.Rack != view.Rack {
		t.Error("a rejected play changed the game")
	}
}

func TestExchangeRejections(t *testing.T) {
	s, _, _ := newService(t)
	view, err := s.Create(ctx, bot.Easy, seedPointer(1))
	if err != nil {
		t.Fatal(err)
	}
	var rejection *Rejection
	if _, err := s.Exchange(ctx, view.ID, "1"); !errors.As(err, &rejection) || rejection.Code != "cannot_exchange" {
		t.Errorf("err = %v", err)
	}
	view, err = s.Exchange(ctx, view.ID, view.Rack[:1])
	if err != nil {
		t.Fatal(err)
	}
	if view.Turns[0].Kind != "exchange" || view.Turns[0].ExchangedCount != 1 || view.Turns[0].Exchanged == "" {
		t.Errorf("turn = %+v", view.Turns[0])
	}
}

func TestPassGetsABotReply(t *testing.T) {
	s, _, _ := newService(t)
	view, err := s.Create(ctx, bot.Medium, seedPointer(1))
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.Pass(ctx, view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Turns[0].Kind != "pass" || len(view.Turns) != 2 {
		t.Errorf("turns = %+v", view.Turns)
	}
}

func TestUnknownGame(t *testing.T) {
	s, _, _ := newService(t)
	if _, err := s.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("get: %v", err)
	}
	if _, err := s.Pass(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("pass: %v", err)
	}
	if _, err := s.GCG(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("gcg: %v", err)
	}
}

// staleStore always finds the game has moved on, as a second tab would cause.
type staleStore struct{ Store }

func (staleStore) Update(context.Context, *Record) error { return ErrConflict }

func TestConcurrentChangeIsReported(t *testing.T) {
	bots, err := bot.Default()
	if err != nil {
		t.Fatal(err)
	}
	s := New(bots, staleStore{NewMemoryStore()})
	view, err := s.Create(ctx, bot.Easy, seedPointer(1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pass(ctx, view.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("err = %v", err)
	}
}

func TestMemoryStoreChecksVersion(t *testing.T) {
	s, _, store := newService(t)
	view, err := s.Create(ctx, bot.Easy, seedPointer(1))
	if err != nil {
		t.Fatal(err)
	}
	stale, err := store.Load(ctx, view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pass(ctx, view.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(ctx, stale); !errors.Is(err, ErrConflict) {
		t.Errorf("err = %v", err)
	}
}

func TestFullGameSavesARecordWithEveryRack(t *testing.T) {
	s, bots, store := newService(t)
	for _, level := range []bot.Level{bot.Easy, bot.Medium, bot.Hard} {
		view, err := s.Create(ctx, level, seedPointer(3))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.GCG(ctx, view.ID); !errors.Is(err, ErrNotFinished) {
			t.Errorf("gcg before the end: %v", err)
		}
		view = finishGame(t, s, bots, view)

		if view.Result == nil || view.Result.Outcome == "" {
			t.Fatalf("result = %+v", view.Result)
		}
		if view.CanExchange {
			t.Error("can't exchange in a finished game")
		}
		last := view.Turns[len(view.Turns)-1]
		adjustments := view.Result.Adjustments
		if last.Player == "you" && view.Scores.You != last.Total+adjustments.You {
			t.Errorf("final score %d, last total %d, adjustment %d", view.Scores.You, last.Total, adjustments.You)
		}

		record, err := s.GCG(ctx, view.ID)
		if err != nil {
			t.Fatal(err)
		}
		stored, err := store.Load(ctx, view.ID)
		if err != nil {
			t.Fatal(err)
		}
		// One line per turn with the rack held on that turn, for both players.
		for _, turn := range stored.State.Turns {
			nick := "Player"
			if turn.Player == botSeat {
				nick = "Rackwise_" + string(level)
			}
			if !strings.Contains(record, fmt.Sprintf(">%s: %s ", nick, turn.Rack)) {
				t.Errorf("record is missing %s's rack %s:\n%s", nick, turn.Rack, record)
			}
		}
		if _, err := s.Pass(ctx, view.ID); !errors.Is(err, ErrGameOver) {
			t.Errorf("pass after the end: %v", err)
		}
	}
}

func TestViewNeverRevealsHiddenTiles(t *testing.T) {
	s, bots, store := newService(t)
	view, err := s.Create(ctx, bot.Hard, seedPointer(5))
	if err != nil {
		t.Fatal(err)
	}
	for turn := 0; view.Status == "active" && turn < 12; turn++ {
		text, err := json.Marshal(view)
		if err != nil {
			t.Fatal(err)
		}
		stored, err := store.Load(ctx, view.ID)
		if err != nil {
			t.Fatal(err)
		}
		// A full rack or bag written out would appear as this exact string.
		if strings.Contains(string(text), `"`+stored.State.Racks[botSeat]+`"`) {
			t.Errorf("turn %d: the bot's rack is in the response: %s", turn, text)
		}
		// Short bags would match by chance, since their letters appear on the board.
		if len(stored.State.Bag) >= 8 && strings.Contains(string(text), stored.State.Bag) {
			t.Errorf("turn %d: the bag is in the response", turn)
		}
		var fields map[string]any
		if err := json.Unmarshal(text, &fields); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"seed", "state", "bag", "bot_rack", "racks"} {
			if _, ok := fields[name]; ok {
				t.Errorf("turn %d: response has a %q field", turn, name)
			}
		}

		tiles := bestPlay(t, bots, view)
		if tiles == nil {
			view, err = s.Exchange(ctx, view.ID, view.Rack)
		} else {
			view, err = s.Play(ctx, view.ID, tiles)
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range view.Turns {
			if line.Player == "bot" && line.Exchanged != "" {
				t.Errorf("the bot's exchanged tiles are visible: %+v", line)
			}
		}
	}
}

func TestSameSeedAndMovesReplayTheSameGame(t *testing.T) {
	s, bots, _ := newService(t)
	play := func() *View {
		view, err := s.Create(ctx, bot.Easy, seedPointer(9))
		if err != nil {
			t.Fatal(err)
		}
		return finishGame(t, s, bots, view)
	}
	first, second := play(), play()
	if first.Scores != second.Scores || strings.Join(first.Board, "") != strings.Join(second.Board, "") {
		t.Error("the same seed and moves gave different games")
	}
}

// TestPostgresStore runs against a real database when one is configured, in its
// own schema so that it leaves no trace in the games table.
func TestPostgresStore(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schemaName := fmt.Sprintf("test_%d", os.Getpid())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schemaName); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schemaName+" CASCADE"); err != nil {
			t.Error(err)
		}
		if err := admin.Close(ctx); err != nil {
			t.Error(err)
		}
	})

	separator := "?"
	if strings.Contains(url, "?") {
		separator = "&"
	}
	store, err := OpenPostgres(ctx, url+separator+"search_path="+schemaName)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	// Opening a second time must be harmless: this happens at every engine start.
	again, err := OpenPostgres(ctx, url+separator+"search_path="+schemaName)
	if err != nil {
		t.Fatal(err)
	}
	again.Close()

	bots, err := bot.Default()
	if err != nil {
		t.Fatal(err)
	}
	s := New(bots, store)
	view, err := s.Create(ctx, bot.Hard, seedPointer(21))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("get missing: %v", err)
	}

	stale, err := store.Load(ctx, view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Seed != 21 || stale.Level != bot.Hard || len(stale.State.Racks[botSeat]) != game.RackSize {
		t.Errorf("loaded = %+v", stale)
	}

	view = finishGame(t, s, bots, view)
	if err := store.Update(ctx, stale); !errors.Is(err, ErrConflict) {
		t.Errorf("stale update: %v", err)
	}
	missing := *stale
	missing.ID = "missing"
	if err := store.Update(ctx, &missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing: %v", err)
	}

	record, err := s.GCG(ctx, view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(record, "#character-encoding UTF-8\n") {
		t.Errorf("record = %q", record)
	}

	var status string
	var personScore, botScore int
	var finished bool
	err = admin.QueryRow(ctx,
		fmt.Sprintf(`SELECT status, person_score, bot_score, finished_at IS NOT NULL FROM %s.games WHERE id = $1`, schemaName),
		view.ID).Scan(&status, &personScore, &botScore, &finished)
	if err != nil {
		t.Fatal(err)
	}
	if status != "finished" || !finished || personScore != view.Scores.You || botScore != view.Scores.Bot {
		t.Errorf("row: %s, %d, %d, %v; view scores %+v", status, personScore, botScore, finished, view.Scores)
	}
}
