package movegen

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"rackwise/engine/data"
	"rackwise/engine/internal/game"
	"rackwise/engine/internal/lexicon"
)

var enable = sync.OnceValue(func() *lexicon.Lexicon {
	lex, err := lexicon.Parse(data.Enable)
	if err != nil {
		panic(err)
	}
	return lex
})

// mapDict checks words without the DAWG, so the brute-force test shares no word
// lookup code with the generator.
type mapDict map[string]bool

func (d mapDict) Contains(word string) bool { return d[word] }

func placementKey(tiles []game.Placement) string {
	parts := make([]string, len(tiles))
	for i, t := range tiles {
		parts[i] = fmt.Sprintf("%02d,%02d,%c", t.Row, t.Col, t.Letter)
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

// bruteForce finds legal plays the slow way: it lays every sequence of rack tiles
// from every free square in both directions, and asks game.Validate about each.
// It uses no anchors, cross-checks or graph, so it can't share the generator's
// mistakes. It does share the scoring routine, which has its own hand-computed
// tests.
func bruteForce(b *game.Board, rack string, dict game.Dictionary) map[string]int {
	found := map[string]int{}
	var place func(row, col, dr, dc int, tiles []game.Placement, remaining string)
	place = func(row, col, dr, dc int, tiles []game.Placement, remaining string) {
		if len(tiles) > 0 {
			if move, err := game.Validate(b, rack, tiles, dict); err == nil {
				found[placementKey(tiles)] = move.Score
			}
		}
		for b.Occupied(row, col) {
			row, col = row+dr, col+dc
		}
		if row >= game.BoardSize || col >= game.BoardSize {
			return
		}
		tried := map[byte]bool{}
		for i := range len(remaining) {
			tile := remaining[i]
			if tried[tile] {
				continue
			}
			tried[tile] = true
			rest := remaining[:i] + remaining[i+1:]
			letters := []byte{tile}
			if tile == game.Blank {
				letters = letters[:0]
				for letter := byte('a'); letter <= 'z'; letter++ {
					letters = append(letters, letter)
				}
			}
			for _, letter := range letters {
				next := append(slices.Clone(tiles), game.Placement{Row: row, Col: col, Letter: letter})
				place(row+dr, col+dc, dr, dc, next, rest)
			}
		}
	}
	for r := range game.BoardSize {
		for c := range game.BoardSize {
			if b[r][c] == 0 {
				place(r, c, 0, 1, nil, rack)
				place(r, c, 1, 0, nil, rack)
			}
		}
	}
	return found
}

func compareWithBruteForce(t *testing.T, b *game.Board, rack string, lex *lexicon.Lexicon, dict game.Dictionary) {
	t.Helper()
	moves := Generate(b, rack, lex)

	generated := map[string]int{}
	for _, move := range moves {
		key := placementKey(move.Tiles)
		if _, seen := generated[key]; seen {
			t.Errorf("rack %s: play listed twice: %s", rack, key)
		}
		generated[key] = move.Score
	}
	expected := bruteForce(b, rack, dict)

	for key, score := range expected {
		got, ok := generated[key]
		if !ok {
			t.Errorf("rack %s: missed play %s\n%s", rack, key, strings.Join(b.Rows(), "\n"))
		} else if got != score {
			t.Errorf("rack %s: play %s scored %d, want %d", rack, key, got, score)
		}
	}
	for key := range generated {
		if _, ok := expected[key]; !ok {
			t.Errorf("rack %s: illegal play %s\n%s", rack, key, strings.Join(b.Rows(), "\n"))
		}
	}
}

// fixtureDictionary is a sample of ENABLE, small enough for quick games yet dense
// enough to produce crowded boards. It keeps every word of 2 or 3 letters.
func fixtureDictionary(seed uint64) (*lexicon.Lexicon, mapDict) {
	rng := rand.New(rand.NewPCG(seed, 0))
	dict := mapDict{}
	var words []string
	for _, word := range strings.Fields(string(data.Enable)) {
		if len(word) <= 3 || (len(word) <= 7 && rng.IntN(20) == 0) {
			word = strings.ToUpper(word)
			dict[word] = true
			words = append(words, word)
		}
	}
	lex, err := lexicon.New(words)
	if err != nil {
		panic(err)
	}
	return lex, dict
}

// randomRack draws from the real tile set.
func randomRack(rng *rand.Rand, size int) string {
	tiles := game.FullTileSet()
	rng.Shuffle(len(tiles), func(i, j int) { tiles[i], tiles[j] = tiles[j], tiles[i] })
	return string(tiles[:size])
}

// buildPosition plays random legal moves to reach a realistic board.
func buildPosition(rng *rand.Rand, lex *lexicon.Lexicon, plays int) *game.Board {
	board := &game.Board{}
	for range plays {
		moves := Generate(board, randomRack(rng, game.RackSize), lex)
		if len(moves) > 0 {
			board.Place(moves[rng.IntN(len(moves))].Tiles)
		}
	}
	return board
}

func TestMatchesBruteForceOnRandomPositions(t *testing.T) {
	lex, dict := fixtureDictionary(11)
	rng := rand.New(rand.NewPCG(11, 1))
	positions := 30
	if testing.Short() {
		positions = 6
	}
	for i := range positions {
		board := buildPosition(rng, lex, i%15)
		rack := randomRack(rng, 3+i%3)
		compareWithBruteForce(t, board, rack, lex, dict)
	}
}

func TestMatchesBruteForceOnFullDictionary(t *testing.T) {
	dict := mapDict{}
	for _, word := range strings.Fields(string(data.Enable)) {
		dict[strings.ToUpper(word)] = true
	}
	rng := rand.New(rand.NewPCG(12, 1))
	for i := range 8 {
		board := buildPosition(rng, enable(), 3+i*2)
		compareWithBruteForce(t, board, randomRack(rng, 4), enable(), dict)
	}
}

func TestMatchesBruteForceOnEmptyBoardWithBlanks(t *testing.T) {
	lex, dict := fixtureDictionary(13)
	for _, rack := range []string{"AB?", "ET?", "Q?I", "??A"} {
		compareWithBruteForce(t, &game.Board{}, rack, lex, dict)
	}
}

func TestMatchesBruteForceAtBoardEdges(t *testing.T) {
	lex, dict := fixtureDictionary(14)
	// Words touching every edge test the ends of both search directions.
	var board game.Board
	board.Place([]game.Placement{
		{Row: 0, Col: 0, Letter: 'A'}, {Row: 0, Col: 14, Letter: 'A'},
		{Row: 14, Col: 0, Letter: 'A'}, {Row: 14, Col: 14, Letter: 'A'},
		{Row: 0, Col: 7, Letter: 'E'}, {Row: 7, Col: 0, Letter: 'e'},
		{Row: 14, Col: 7, Letter: 'E'}, {Row: 7, Col: 14, Letter: 'E'},
	})
	compareWithBruteForce(t, &board, "ATES", lex, dict)
}

func wordsFromRack(word, rack string) bool {
	remaining := []byte(rack)
	for i := range len(word) {
		j := slices.Index(remaining, word[i])
		if j < 0 {
			return false
		}
		remaining = slices.Delete(remaining, j, j+1)
	}
	return true
}

func TestFirstMoveFindsEveryAnagram(t *testing.T) {
	const rack = "AEINRST"

	// Worked out from the raw list, without the graph.
	want := map[string]bool{}
	for _, word := range strings.Fields(string(data.Enable)) {
		word = strings.ToUpper(word)
		if wordsFromRack(word, rack) {
			want[word] = true
		}
	}
	for _, bingo := range []string{"NASTIER", "RETAINS", "RETINAS", "RETSINA", "STAINER"} {
		if !want[bingo] {
			t.Fatalf("expected %s among the anagrams", bingo)
		}
	}

	got := map[string]bool{}
	bingos := 0
	for _, move := range Generate(&game.Board{}, rack, enable()) {
		got[move.Word] = true
		if move.Bingo {
			bingos++
		}
		if !slices.ContainsFunc(move.Tiles, func(p game.Placement) bool {
			return p.Row == game.Center && p.Col == game.Center
		}) {
			t.Errorf("%s does not cover the centre", move.Word)
		}
	}
	if len(got) != len(want) {
		t.Errorf("found %d words, want %d", len(got), len(want))
	}
	for word := range want {
		if !got[word] {
			t.Errorf("missed %s", word)
		}
	}
	if bingos == 0 {
		t.Error("expected bingos")
	}
}

func TestFirstMoveBestScoreIsKnown(t *testing.T) {
	// All seven tiles are worth 1. The best placement crosses both a double word
	// and a double letter, as RETAINS does from column 7: (6 + 2) x 2 + 40 = 56.
	moves := Generate(&game.Board{}, "AEINRST", enable())
	best := 0
	for _, move := range moves {
		if move.Bingo && move.Score > best {
			best = move.Score
		}
	}
	if best != 56 {
		t.Errorf("best bingo = %d, want 56", best)
	}
	if !slices.IsSortedFunc(moves, func(a, b *game.Move) int { return b.Score - a.Score }) {
		t.Error("moves should be sorted by score")
	}
}

func TestQIIsPlayableAndZAIsNot(t *testing.T) {
	lex, err := lexicon.Parse(data.Dictionary())
	if err != nil {
		t.Fatal(err)
	}
	moves := Generate(&game.Board{}, "AIQZ", lex)
	if findMove(moves, "QI", 2) == nil {
		t.Error("QI should be generated")
	}
	if move := findMove(moves, "ZA", 2); move != nil {
		t.Errorf("ZA should not be generated: %+v", move)
	}
}

func findMove(moves []*game.Move, word string, tiles int) *game.Move {
	for _, move := range moves {
		if move.Word == word && len(move.Tiles) == tiles {
			return move
		}
	}
	return nil
}

func TestBlanksMakeSeparatePlays(t *testing.T) {
	moves := Generate(&game.Board{}, "?AB", enable())
	// Real B and A on plain squares: 4 + 1.
	if m := findMove(moves, "BA", 2); m == nil || m.Score != 5 {
		t.Errorf("BA: %+v", m)
	}
	// Blank as B: 0 + 1.
	if m := findMove(moves, "bA", 2); m == nil || m.Score != 1 {
		t.Errorf("bA: %+v", m)
	}
	// Blank as A: 4 + 0.
	if m := findMove(moves, "Ba", 2); m == nil || m.Score != 4 {
		t.Errorf("Ba: %+v", m)
	}
}

func TestHookAndParallelPlays(t *testing.T) {
	// RAT on row 7, columns 6 to 8.
	var board game.Board
	board.Place([]game.Placement{{Row: 7, Col: 6, Letter: 'R'}, {Row: 7, Col: 7, Letter: 'A'}, {Row: 7, Col: 8, Letter: 'T'}})
	moves := Generate(&board, "OS", enable())

	// S hooks RATS: 1+1+1+1, no premium on that square.
	if m := findMove(moves, "RATS", 1); m == nil || m.Score != 4 {
		t.Errorf("RATS: %+v", m)
	}
	// SO down from the hook, with O on a triple letter: S 1 + O 3 = 4, plus the
	// cross-word RATS for 4.
	i := slices.IndexFunc(moves, func(m *game.Move) bool {
		return m.Word == "SO" && m.Row == 7 && m.Col == 9 && m.Dir == game.Down
	})
	if i < 0 {
		t.Fatal("SO from the hook was not found")
	}
	if m := moves[i]; m.Score != 8 || !slices.Equal(m.CrossWords, []string{"RATS"}) {
		t.Errorf("SO: %+v", m)
	}
	// Every cross-word offered must be real.
	for _, move := range moves {
		for _, word := range move.CrossWords {
			if !enable().Contains(strings.ToUpper(word)) {
				t.Errorf("%s made invalid cross-word %s", move.Word, word)
			}
		}
	}
}

func TestPlayedBlankOnBoardCountsAsItsLetter(t *testing.T) {
	var board game.Board
	board.Place([]game.Placement{{Row: 7, Col: 7, Letter: 'c'}, {Row: 7, Col: 8, Letter: 'a'}, {Row: 7, Col: 9, Letter: 't'}})
	// All old tiles are blanks worth 0, and S adds 1.
	if m := findMove(Generate(&board, "S", enable()), "catS", 1); m == nil || m.Score != 1 {
		t.Errorf("catS: %+v", m)
	}
}

func TestNoTilesNoMoves(t *testing.T) {
	if moves := Generate(&game.Board{}, "", enable()); len(moves) != 0 {
		t.Errorf("got %d moves from an empty rack", len(moves))
	}
}

func TestOutputIsDeterministic(t *testing.T) {
	rng := rand.New(rand.NewPCG(15, 1))
	board := buildPosition(rng, enable(), 6)
	key := func(moves []*game.Move) []string {
		keys := make([]string, len(moves))
		for i, move := range moves {
			keys[i] = placementKey(move.Tiles)
		}
		return keys
	}
	if !slices.Equal(key(Generate(board, "AEIRST?", enable())), key(Generate(board, "AEIRST?", enable()))) {
		t.Error("the same position gave a different move order")
	}
}

func benchmarkPosition() *game.Board {
	return buildPosition(rand.New(rand.NewPCG(16, 1)), enable(), 10)
}

func BenchmarkGenerateMidgame(b *testing.B) {
	board := benchmarkPosition()
	for b.Loop() {
		Generate(board, "AEILNRT", enable())
	}
}

func BenchmarkGenerateDoubleBlank(b *testing.B) {
	board := benchmarkPosition()
	for b.Loop() {
		Generate(board, "??AEIRT", enable())
	}
}
