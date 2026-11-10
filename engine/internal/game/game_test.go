package game

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// Every expected score in this file is worked out by hand from the layout and
// tile values in rules.go.

type wordSet map[string]bool

func (w wordSet) Contains(word string) bool { return w[word] }

func newWords(words ...string) wordSet {
	set := wordSet{}
	for _, word := range words {
		set[word] = true
	}
	return set
}

// line builds placements for a word, skipping '.' for squares already filled.
func line(row, col int, dir Direction, letters string) []Placement {
	var tiles []Placement
	for i := range len(letters) {
		if letters[i] == '.' {
			continue
		}
		t := Placement{Row: row, Col: col + i, Letter: letters[i]}
		if dir == Down {
			t = Placement{Row: row + i, Col: col, Letter: letters[i]}
		}
		tiles = append(tiles, t)
	}
	return tiles
}

func boardWith(tiles ...[]Placement) *Board {
	var b Board
	for _, group := range tiles {
		b.Place(group)
	}
	return &b
}

func TestLayoutIsSymmetricWithExpectedPremiums(t *testing.T) {
	counts := map[Premium]int{}
	for r := range BoardSize {
		for c := range BoardSize {
			p := PremiumAt(r, c)
			counts[p]++
			if PremiumAt(c, r) != p || PremiumAt(BoardSize-1-r, c) != p || PremiumAt(r, BoardSize-1-c) != p {
				t.Errorf("square %d,%d is not symmetric", r, c)
			}
		}
	}
	if PremiumAt(Center, Center) != NoPremium {
		t.Error("the centre square should have no premium")
	}
	if counts[TripleWord] != 8 {
		t.Errorf("triple word squares = %d, want 8", counts[TripleWord])
	}
	for _, square := range [][2]int{{0, 3}, {0, 11}, {3, 0}, {3, 14}, {11, 0}, {11, 14}, {14, 3}, {14, 11}} {
		if PremiumAt(square[0], square[1]) != TripleWord {
			t.Errorf("square %v should be a triple word", square)
		}
	}
}

func TestTileSet(t *testing.T) {
	tiles := FullTileSet()
	if len(tiles) != 100 {
		t.Errorf("tile set has %d tiles, want 100", len(tiles))
	}
	counts := map[byte]int{}
	for _, tile := range tiles {
		counts[tile]++
	}
	for tile, want := range map[byte]int{'E': 11, 'A': 9, 'S': 5, 'Q': 1, 'U': 4, Blank: 2} {
		if counts[tile] != want {
			t.Errorf("%c count = %d, want %d", tile, counts[tile], want)
		}
	}
	for tile, want := range map[byte]int{'A': 1, 'D': 2, 'G': 3, 'H': 4, 'W': 5, 'K': 6, 'X': 8, 'Z': 9, 'Q': 10, 'a': 0, Blank: 0} {
		if got := TileValue(tile); got != want {
			t.Errorf("value of %c = %d, want %d", tile, got, want)
		}
	}
}

func TestScoring(t *testing.T) {
	cases := []struct {
		name  string
		board *Board
		tiles []Placement
		score int
		dir   Direction
		word  string
		cross []string
		bingo bool
	}{
		{
			// Z on double letter 18, E on double word, then R O E S.
			// (18+1+1+1+1+1) x 2.
			name:  "double letter and double word",
			board: boardWith(),
			tiles: line(7, 2, Across, "ZEROES"),
			score: 46,
			word:  "ZEROES",
		},
		{
			// X on triple letter 24, plus I.
			name:  "triple letter",
			board: boardWith(),
			tiles: line(0, 0, Across, "XI"),
			score: 25,
			word:  "XI",
		},
		{
			// Blank X on the triple letter is still 0, plus I.
			name:  "blank on letter premium scores nothing",
			board: boardWith(),
			tiles: line(0, 0, Across, "xI"),
			score: 1,
			word:  "xI",
		},
		{
			// (0 + 1) x 2: the blank still triggers the word premium.
			name:  "blank on word premium still multiplies",
			board: boardWith(),
			tiles: line(1, 1, Across, "aT"),
			score: 2,
			word:  "aT",
		},
		{
			// Two triple words multiply to 9. D and F were already there, on
			// double letters that no longer count. 7 tiles adds the bingo.
			// (1+4+3+2+1+4+3+4+1) x 9 + 40.
			name:  "stacked word premiums, used premiums and bingo",
			board: boardWith(line(0, 6, Across, "D"), line(0, 8, Across, "F")),
			tiles: line(0, 3, Across, "ABC.E.GHI"),
			score: 247,
			word:  "ABCDEFGHI",
			bingo: true,
		},
		{
			// I on the double word at column 11, N doubled on the double letter
			// at column 12. (1+1+1+1+1+2+1) x 2 + 40.
			name:  "bingo",
			board: boardWith(),
			tiles: line(7, 7, Across, "RETAINS"),
			score: 56,
			word:  "RETAINS",
			bingo: true,
		},
		{
			// One tile on a double word forms two words, and both are doubled.
			// HE (4+1) x 2 and ED (1+2) x 2.
			name:  "premium counts in both words",
			board: boardWith(line(3, 6, Across, "H"), line(4, 7, Across, "D")),
			tiles: line(3, 7, Across, "E"),
			score: 16,
			word:  "HE",
			cross: []string{"ED"},
		},
		{
			// As above with a blank E. HE 4 x 2 and ED 2 x 2.
			name:  "blank premium counts in both words",
			board: boardWith(line(3, 6, Across, "H"), line(4, 7, Across, "D")),
			tiles: line(3, 7, Across, "e"),
			score: 12,
			word:  "He",
			cross: []string{"eD"},
		},
		{
			// A single tile joining only a column counts as down. X tripled, plus A.
			name:  "single tile down",
			board: boardWith(line(4, 3, Across, "A")),
			tiles: line(3, 3, Across, "X"),
			score: 25,
			dir:   Down,
			word:  "XA",
		},
		{
			// TO 2 under AT, with cross-words AT 2 and TO 2. No premiums.
			name:  "parallel play scores every cross-word",
			board: boardWith(line(7, 7, Across, "AT")),
			tiles: line(8, 7, Across, "TO"),
			score: 6,
			word:  "TO",
			cross: []string{"AT", "TO"},
		},
		{
			// Old tile in the main word scores face value: C A T S across, with
			// no premium under the S.
			name:  "hook adds face value of old tiles",
			board: boardWith(line(7, 6, Across, "CAT")),
			tiles: line(7, 9, Across, "S"),
			score: 6,
			word:  "CATS",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			move := ScorePlay(tc.board, tc.tiles)
			if move.Score != tc.score {
				t.Errorf("score = %d, want %d", move.Score, tc.score)
			}
			if move.Word != tc.word {
				t.Errorf("word = %q, want %q", move.Word, tc.word)
			}
			if move.Dir != tc.dir {
				t.Errorf("direction = %v, want %v", move.Dir, tc.dir)
			}
			if !slices.Equal(move.CrossWords, tc.cross) {
				t.Errorf("cross-words = %v, want %v", move.CrossWords, tc.cross)
			}
			if move.Bingo != tc.bingo {
				t.Errorf("bingo = %v, want %v", move.Bingo, tc.bingo)
			}
		})
	}
}

func TestSixTilesIsNotABingo(t *testing.T) {
	if ScorePlay(boardWith(), line(7, 2, Across, "ZEROES")).Bingo {
		t.Error("6 tiles should not be a bingo")
	}
}

func TestPosition(t *testing.T) {
	across := &Move{Row: 7, Col: 7, Dir: Across}
	if got := across.Position(); got != "8H" {
		t.Errorf("across position = %q, want 8H", got)
	}
	down := &Move{Row: 7, Col: 7, Dir: Down}
	if got := down.Position(); got != "H8" {
		t.Errorf("down position = %q, want H8", got)
	}
}

func TestValidate(t *testing.T) {
	dict := newWords("AT", "CAT", "CATS", "TO", "SO", "OAT", "OATS", "XI")
	withCat := boardWith(line(7, 6, Across, "CAT"))

	cases := []struct {
		name  string
		board *Board
		rack  string
		tiles []Placement
		want  ErrorCode
	}{
		{"legal first play", boardWith(), "AT", line(7, 7, Across, "AT"), ""},
		{"legal hook", withCat, "S", line(7, 9, Across, "S"), ""},
		{"legal play through a tile", withCat, "OTS", line(6, 7, Down, "O.TS"), ""},
		{"legal with a blank", boardWith(), "?T", line(7, 7, Across, "aT"), ""},
		{"no tiles", boardWith(), "AT", nil, ErrNoTiles},
		{"bad letter", boardWith(), "AT", []Placement{{7, 7, '5'}}, ErrBadLetter},
		{"off the board", boardWith(), "AT", []Placement{{7, 15, 'A'}}, ErrOutOfBounds},
		{"negative square", boardWith(), "AT", []Placement{{-1, 7, 'A'}}, ErrOutOfBounds},
		{"occupied", withCat, "S", line(7, 6, Across, "S"), ErrOccupied},
		{"same square twice", boardWith(), "AA", []Placement{{7, 7, 'A'}, {7, 7, 'A'}}, ErrDuplicate},
		{"not in a line", boardWith(), "AT", []Placement{{7, 7, 'A'}, {8, 8, 'T'}}, ErrNotInLine},
		{"gap", boardWith(), "AT", []Placement{{7, 7, 'A'}, {7, 9, 'T'}}, ErrGap},
		{"tile not on rack", boardWith(), "AB", line(7, 7, Across, "AT"), ErrNotOnRack},
		{"blank not on rack", boardWith(), "AT", line(7, 7, Across, "aT"), ErrNotOnRack},
		{"too many of a tile", boardWith(), "AT", line(7, 7, Across, "AA"), ErrNotOnRack},
		{"first play misses centre", boardWith(), "AT", line(0, 0, Across, "AT"), ErrNotCentered},
		{"first play one tile", boardWith(), "A", line(7, 7, Across, "A"), ErrTooShort},
		{"not connected", withCat, "AT", line(0, 0, Across, "AT"), ErrNotConnected},
		{"invalid main word", boardWith(), "TT", line(7, 7, Across, "TT"), ErrInvalidWord},
		{"invalid hook", withCat, "X", line(7, 9, Across, "X"), ErrInvalidWord},
		{"invalid cross-word", withCat, "XI", line(8, 6, Across, "XI"), ErrInvalidWord},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Validate(tc.board, tc.rack, tc.tiles, dict)
			if got := CodeOf(err); got != tc.want {
				t.Errorf("code = %q, want %q (error: %v)", got, tc.want, err)
			}
		})
	}
}

func TestParseBoardAndRack(t *testing.T) {
	var original Board
	original.Place(line(7, 6, Across, "CaT"))
	parsed, err := ParseBoard(original.Rows())
	if err != nil || *parsed != original {
		t.Errorf("board did not round trip: %v", err)
	}

	short := original.Rows()
	short[3] = "..."
	if _, err := ParseBoard(short); err == nil {
		t.Error("expected an error for a short row")
	}
	if _, err := ParseBoard(original.Rows()[:14]); err == nil {
		t.Error("expected an error for a missing row")
	}
	bad := original.Rows()
	bad[0] = "1.............."
	if _, err := ParseBoard(bad); err == nil {
		t.Error("expected an error for a bad character")
	}

	if rack, err := ParseRack("TA?"); err != nil || rack != "?AT" {
		t.Errorf("rack = %q, %v", rack, err)
	}
	for _, rack := range []string{"ABCDEFGH", "ab", "A1"} {
		if _, err := ParseRack(rack); err == nil {
			t.Errorf("expected an error for rack %q", rack)
		}
	}
}

// fixedGame deals known racks and a known bag.
func fixedGame(dict Dictionary, rack0, rack1, bag string) *Game {
	g := NewGame(dict, rand.New(rand.NewPCG(1, 1)))
	g.Racks = [2]string{rack0, rack1}
	g.Bag.tiles = []byte(bag)
	return g
}

func TestNewGameDeals(t *testing.T) {
	g := NewGame(newWords(), rand.New(rand.NewPCG(1, 1)))
	if len(g.Racks[0]) != RackSize || len(g.Racks[1]) != RackSize {
		t.Errorf("racks = %q, %q", g.Racks[0], g.Racks[1])
	}
	if g.Bag.Remaining() != 86 {
		t.Errorf("bag = %d, want 86", g.Bag.Remaining())
	}
}

func TestPlayScoresDrawsAndSwitchesPlayer(t *testing.T) {
	g := fixedGame(newWords("AT"), "ATXXXXX", "ZZZZZZZ", "QJK")
	if _, err := g.Play(line(7, 7, Across, "AT")); err != nil {
		t.Fatal(err)
	}
	if g.Scores[0] != 2 {
		t.Errorf("score = %d, want 2", g.Scores[0])
	}
	if g.Racks[0] != "JQXXXXX" {
		t.Errorf("rack = %q, want JQXXXXX", g.Racks[0])
	}
	if g.Bag.Remaining() != 1 || g.Current != 1 || g.Over {
		t.Errorf("bag = %d, current = %d, over = %v", g.Bag.Remaining(), g.Current, g.Over)
	}
	turn := g.Turns[0]
	if turn.Rack != "ATXXXXX" || turn.Score != 2 || turn.Kind != TurnPlay || turn.BagAfter != 1 {
		t.Errorf("turn record = %+v", turn)
	}
}

func TestIllegalPlayChangesNothing(t *testing.T) {
	g := fixedGame(newWords("AT"), "TT", "Z", "Q")
	if _, err := g.Play(line(7, 7, Across, "TT")); err == nil {
		t.Fatal("expected an error")
	}
	if !g.Board.IsEmpty() || g.Racks[0] != "TT" || g.Current != 0 || len(g.Turns) != 0 {
		t.Error("an illegal play changed the game")
	}
}

func TestGoingOutAppliesEndPenalties(t *testing.T) {
	g := fixedGame(newWords("AT"), "AT", "QZ", "")
	if _, err := g.Play(line(7, 7, Across, "AT")); err != nil {
		t.Fatal(err)
	}
	if !g.Over {
		t.Fatal("the game should be over")
	}
	// AT scores 2. Q and Z are worth 19, lost by player 1 and gained by player 0.
	if g.Scores != [2]int{21, -19} {
		t.Errorf("scores = %v, want [21 -19]", g.Scores)
	}
	if g.EndAdjustments != [2]int{19, -19} {
		t.Errorf("adjustments = %v, want [19 -19]", g.EndAdjustments)
	}
}

func TestPlayedBlanksDoNotCountInEndPenalty(t *testing.T) {
	g := fixedGame(newWords("AT"), "AT", "?Q", "")
	if _, err := g.Play(line(7, 7, Across, "AT")); err != nil {
		t.Fatal(err)
	}
	// The unplayed blank is worth nothing, so only Q's 10 moves.
	if g.Scores != [2]int{12, -10} {
		t.Errorf("scores = %v, want [12 -10]", g.Scores)
	}
}

func TestEmptyingRackWithBagLeftDoesNotEndGame(t *testing.T) {
	g := fixedGame(newWords("AT"), "AT", "QZ", "XX")
	if _, err := g.Play(line(7, 7, Across, "AT")); err != nil {
		t.Fatal(err)
	}
	if g.Over {
		t.Error("the game should go on: the player drew new tiles")
	}
}

func TestSixScorelessTurnsEndTheGame(t *testing.T) {
	g := fixedGame(newWords(), "AT", "QZ", "XXXXXXXX")
	for i := range MaxScorelessTurns - 1 {
		if err := g.Pass(); err != nil {
			t.Fatal(err)
		}
		if g.Over {
			t.Fatalf("game ended after %d turns", i+1)
		}
	}
	if err := g.Pass(); err != nil {
		t.Fatal(err)
	}
	if !g.Over {
		t.Fatal("the game should be over")
	}
	// Each player only loses their own tiles' value.
	if g.Scores != [2]int{-2, -19} {
		t.Errorf("scores = %v, want [-2 -19]", g.Scores)
	}
	if err := g.Pass(); err != ErrGameOver {
		t.Errorf("pass after the end: %v", err)
	}
	if _, err := g.Play(line(7, 7, Across, "AT")); err != ErrGameOver {
		t.Errorf("play after the end: %v", err)
	}
}

func TestScoringPlayResetsScorelessRun(t *testing.T) {
	g := fixedGame(newWords("AT"), "AT", "QZ", "XXXXXXXX")
	for range MaxScorelessTurns - 2 {
		if err := g.Pass(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := g.Play(line(7, 7, Across, "AT")); err != nil {
		t.Fatal(err)
	}
	for range MaxScorelessTurns - 1 {
		if err := g.Pass(); err != nil {
			t.Fatal(err)
		}
	}
	if g.Over {
		t.Error("the run should have restarted after the scoring play")
	}
}

func TestExchange(t *testing.T) {
	g := fixedGame(newWords(), "ABCDEFG", "ZZZZZZZ", "QQQQQQQQ")
	if err := g.Exchange("AB"); err != nil {
		t.Fatal(err)
	}
	if g.Racks[0] != "CDEFGQQ" {
		t.Errorf("rack = %q, want CDEFGQQ", g.Racks[0])
	}
	if g.Bag.Remaining() != 8 {
		t.Errorf("bag = %d, want 8", g.Bag.Remaining())
	}
	if !slices.Contains(g.Bag.tiles, 'A') || !slices.Contains(g.Bag.tiles, 'B') {
		t.Error("exchanged tiles should be back in the bag")
	}
	if g.Current != 1 || g.Turns[0].Exchanged != "AB" || g.Scores[0] != 0 {
		t.Errorf("after exchange: current %d, turn %+v, score %d", g.Current, g.Turns[0], g.Scores[0])
	}
}

func TestExchangeNeedsSevenInBag(t *testing.T) {
	g := fixedGame(newWords(), "ABCDEFG", "ZZZZZZZ", "QQQQQQ")
	if err := g.Exchange("A"); err == nil {
		t.Error("expected an error with 6 tiles in the bag")
	}
	if g.Racks[0] != "ABCDEFG" || g.Current != 0 {
		t.Error("a refused exchange changed the game")
	}

	g = fixedGame(newWords(), "ABCDEFG", "ZZZZZZZ", "QQQQQQQ")
	if err := g.Exchange("A"); err != nil {
		t.Errorf("7 tiles in the bag should be enough: %v", err)
	}
}

func TestExchangeRejectsTilesNotHeld(t *testing.T) {
	g := fixedGame(newWords(), "AB", "ZZZZZZZ", "QQQQQQQQ")
	if err := g.Exchange("X"); err == nil {
		t.Error("expected an error for a tile not on the rack")
	}
	if err := g.Exchange(""); err == nil {
		t.Error("expected an error for no tiles")
	}
}

func TestSameSeedGivesSameGame(t *testing.T) {
	first := NewGame(newWords(), rand.New(rand.NewPCG(7, 7)))
	second := NewGame(newWords(), rand.New(rand.NewPCG(7, 7)))
	if first.Racks != second.Racks || !slices.Equal(first.Bag.tiles, second.Bag.tiles) {
		t.Error("the same seed dealt different games")
	}
}
