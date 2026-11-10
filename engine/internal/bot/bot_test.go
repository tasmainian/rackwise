package bot

import (
	"math/rand/v2"
	"strings"
	"testing"

	"rackwise/engine/internal/eval"
	"rackwise/engine/internal/game"
	"rackwise/engine/internal/lexicon"
)

// fixtureBots uses tiny word lists, so each test's choice is easy to work out.
func fixtureBots(t *testing.T, words, common []string, leavesJSON string, lanePenalty float64) *Bots {
	t.Helper()
	lex, err := lexicon.New(words)
	if err != nil {
		t.Fatal(err)
	}
	commonLex, err := lexicon.New(common)
	if err != nil {
		t.Fatal(err)
	}
	leaves, err := eval.ParseLeaves([]byte(leavesJSON))
	if err != nil {
		t.Fatal(err)
	}
	return &Bots{Lex: lex, Common: commonLex, Eval: &eval.Evaluator{Leaves: leaves, LanePenalty: lanePenalty}}
}

func rng() *rand.Rand { return rand.New(rand.NewPCG(1, 1)) }

func TestParseLevel(t *testing.T) {
	for _, name := range []string{"easy", "medium", "hard"} {
		if _, err := ParseLevel(name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := ParseLevel("expert"); err == nil {
		t.Error("expected an error for an unknown level")
	}
}

func TestMediumPlaysHighestScore(t *testing.T) {
	bots := fixtureBots(t, []string{"AT", "ATS"}, nil, `{"singles":{"S":30}}`, 0)
	action, err := bots.Choose(Medium, &game.Board{}, "ATS", 50, rng())
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != game.TurnPlay || action.Move.Word != "ATS" {
		t.Errorf("action = %+v", action)
	}
	if action.Evaluation.Leave != "" {
		t.Errorf("leave = %q, want none", action.Evaluation.Leave)
	}
}

func TestHardValuesTheLeave(t *testing.T) {
	// ATS scores 3 and keeps nothing. AT scores 2 and keeps an S worth 30.
	bots := fixtureBots(t, []string{"AT", "ATS"}, nil, `{"singles":{"S":30}}`, 0)
	action, err := bots.Choose(Hard, &game.Board{}, "ATS", 50, rng())
	if err != nil {
		t.Fatal(err)
	}
	if action.Move.Word != "AT" || action.Evaluation.Leave != "S" || action.Evaluation.Equity != 32 {
		t.Errorf("move = %s, evaluation = %+v", action.Move.Word, action.Evaluation)
	}
}

func TestHardIgnoresLearnedValuesWithEmptyBag(t *testing.T) {
	// With nothing to draw, keeping the S only costs its point, so play it.
	bots := fixtureBots(t, []string{"AT", "ATS"}, nil, `{"singles":{"S":30}}`, 0)
	action, err := bots.Choose(Hard, &game.Board{}, "ATS", 0, rng())
	if err != nil {
		t.Fatal(err)
	}
	if action.Move.Word != "ATS" {
		t.Errorf("move = %s, want ATS", action.Move.Word)
	}
}

func TestHardAvoidsOpeningLanes(t *testing.T) {
	// Placing the word from column 3 or ending on column 11 hits a double word
	// for 22 points, but puts a tile within reach of triple words in that column.
	words := []string{"ABCDE"}
	medium := fixtureBots(t, words, nil, `{}`, 100)
	action, err := medium.Choose(Medium, &game.Board{}, "ABCDE", 50, rng())
	if err != nil {
		t.Fatal(err)
	}
	if action.Move.Score != 22 {
		t.Errorf("medium scored %d, want 22", action.Move.Score)
	}

	action, err = medium.Choose(Hard, &game.Board{}, "ABCDE", 50, rng())
	if err != nil {
		t.Fatal(err)
	}
	if action.Evaluation.LanesOpened != 0 || action.Move.Score != 11 {
		t.Errorf("hard scored %d, opening %d lanes; want 11 and 0", action.Move.Score, action.Evaluation.LanesOpened)
	}

	noPenalty := fixtureBots(t, words, nil, `{}`, 0)
	action, err = noPenalty.Choose(Hard, &game.Board{}, "ABCDE", 50, rng())
	if err != nil {
		t.Fatal(err)
	}
	if action.Move.Score != 22 {
		t.Errorf("without a penalty hard scored %d, want 22", action.Move.Score)
	}
}

func TestHardExchangesWhenBestPlayIsPoor(t *testing.T) {
	bots := fixtureBots(t, []string{"AT"}, nil, `{"singles":{"S":10,"T":5,"A":-1,"Q":-8}}`, 0)
	bots.ExchangeBelow = 20

	// AT is worth at most 2 + 10 for keeping the S, which is under the threshold.
	action, err := bots.Choose(Hard, &game.Board{}, "ATSQ", 50, rng())
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != game.TurnExchange || action.Exchange != "AQ" {
		t.Errorf("action = %+v, want to exchange A and Q and keep S and T", action)
	}

	// Too few tiles to exchange, so it plays anyway.
	action, err = bots.Choose(Hard, &game.Board{}, "ATSQ", game.MinBagForExchange-1, rng())
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != game.TurnPlay {
		t.Errorf("action = %+v, want a play", action)
	}
}

func TestNoMoveMeansExchangeOrPass(t *testing.T) {
	bots := fixtureBots(t, []string{"AT"}, []string{"AT"}, `{}`, 0)
	for _, level := range []Level{Easy, Medium, Hard} {
		action, err := bots.Choose(level, &game.Board{}, "QQQ", game.MinBagForExchange, rng())
		if err != nil {
			t.Fatal(err)
		}
		if action.Kind != game.TurnExchange || action.Exchange == "" {
			t.Errorf("%s with bag: %+v", level, action)
		}
		action, err = bots.Choose(level, &game.Board{}, "QQQ", game.MinBagForExchange-1, rng())
		if err != nil {
			t.Fatal(err)
		}
		if action.Kind != game.TurnPass {
			t.Errorf("%s without bag: %+v", level, action)
		}
	}
}

func TestEasyOnlyPlaysFamiliarWords(t *testing.T) {
	// QAT scores far more, but isn't familiar.
	bots := fixtureBots(t, []string{"AT", "QAT", "CAT"}, []string{"CAT", "AT"}, `{}`, 0)
	for seed := range 50 {
		action, err := bots.Choose(Easy, &game.Board{}, "ACQT", 50, rand.New(rand.NewPCG(uint64(seed), 0)))
		if err != nil {
			t.Fatal(err)
		}
		if action.Move == nil || action.Move.Word == "QAT" {
			t.Fatalf("seed %d: %+v", seed, action)
		}
	}
}

func TestEasyPlaysAtLeastHalfItsBest(t *testing.T) {
	// CAT and its placements score 5 or more. AT scores 2, under half of the best.
	bots := fixtureBots(t, []string{"AT", "CAT"}, []string{"AT", "CAT"}, `{}`, 0)
	seen := map[int]bool{}
	for seed := range 200 {
		action, err := bots.Choose(Easy, &game.Board{}, "ACT", 50, rand.New(rand.NewPCG(uint64(seed), 0)))
		if err != nil {
			t.Fatal(err)
		}
		seen[action.Move.Score] = true
		if action.Move.Word == "AT" {
			t.Fatalf("seed %d played AT for %d", seed, action.Move.Score)
		}
	}
	if len(seen) == 0 {
		t.Fatal("no plays seen")
	}
}

func TestEasyVariesButIsRepeatableWithASeed(t *testing.T) {
	bots := fixtureBots(t, []string{"CAT", "ACT", "TA", "AT"}, []string{"CAT", "ACT"}, `{}`, 0)
	choose := func(seed uint64) string {
		action, err := bots.Choose(Easy, &game.Board{}, "ACT", 50, rand.New(rand.NewPCG(seed, 0)))
		if err != nil {
			t.Fatal(err)
		}
		return action.Move.Position() + action.Move.Word
	}
	different := map[string]bool{}
	for seed := range uint64(50) {
		first := choose(seed)
		if first != choose(seed) {
			t.Errorf("seed %d gave different plays", seed)
		}
		different[first] = true
	}
	if len(different) < 2 {
		t.Error("the easy bot never varied its play")
	}
}

func TestEasyAllowsUnfamiliarTwoLetterCrossWords(t *testing.T) {
	// Nothing is familiar here, but 2-letter words only need to be legal, so AA is
	// allowed. AAA is 3 letters, so it isn't.
	bots := fixtureBots(t, []string{"AA", "AAA"}, nil, `{}`, 0)
	var board game.Board
	board.Place([]game.Placement{{Row: 7, Col: 7, Letter: 'A'}})
	action, err := bots.Choose(Easy, &board, "AA", 50, rng())
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != game.TurnPlay || action.Move.Word != "AA" {
		t.Errorf("action = %+v", action)
	}
}

// checkGame confirms a finished game obeys the bookkeeping rules.
func checkGame(t *testing.T, g *game.Game) {
	t.Helper()
	if !g.Over {
		t.Error("the game did not finish")
	}

	// Final score = sum of turn scores + end adjustment.
	sums := [2]int{}
	for _, turn := range g.Turns {
		sums[turn.Player] += turn.Score
	}
	for player := range 2 {
		if sums[player]+g.EndAdjustments[player] != g.Scores[player] {
			t.Errorf("player %d: turns %d + adjustment %d != final %d",
				player, sums[player], g.EndAdjustments[player], g.Scores[player])
		}
	}

	// Every tile is on the board, in a rack or in the bag, with blanks tracked.
	onBoard := 0
	for _, row := range g.Board.Rows() {
		onBoard += len(row) - strings.Count(row, ".")
	}
	if total := onBoard + len(g.Racks[0]) + len(g.Racks[1]) + g.Bag.Remaining(); total != 100 {
		t.Errorf("tiles accounted for = %d, want 100", total)
	}
}

func TestSelfPlayFollowsTheRules(t *testing.T) {
	bots, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	levels := []Level{Easy, Medium, Hard}
	for _, a := range levels {
		for _, b := range levels {
			for seed := range uint64(3) {
				// PlayGame goes through game.Play, so an illegal move fails here.
				g, err := PlayGame([2]Agent{{bots, a}, {bots, b}}, seed)
				if err != nil {
					t.Fatalf("%s vs %s: %v", a, b, err)
				}
				checkGame(t, g)
			}
		}
	}
}

func TestSameSeedGivesSameSelfPlayGame(t *testing.T) {
	bots, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	play := func() *game.Game {
		g, err := PlayGame([2]Agent{{bots, Easy}, {bots, Hard}}, 42)
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	first, second := play(), play()
	if first.Scores != second.Scores || first.Board != second.Board || len(first.Turns) != len(second.Turns) {
		t.Error("the same seed gave different games")
	}
}

func TestStrengthOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("plays many games")
	}
	bots, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	wins := 0
	const games = 40
	for i := range games {
		seat := i % 2
		agents := [2]Agent{{bots, Medium}, {bots, Easy}}
		if seat == 1 {
			agents = [2]Agent{{bots, Easy}, {bots, Medium}}
		}
		g, err := PlayGame(agents, uint64(1000+i))
		if err != nil {
			t.Fatal(err)
		}
		if g.Scores[seat] > g.Scores[1-seat] {
			wins++
		}
	}
	if wins < games*3/4 {
		t.Errorf("medium beat easy in %d of %d games", wins, games)
	}
}
