package eval

import (
	"math"
	"testing"

	"rackwise/engine/internal/game"
)

func testLeaves(t *testing.T) *Leaves {
	t.Helper()
	leaves, err := ParseLeaves([]byte(`{
		"singles": {"S": 8, "Q": -7, "U": -3, "?": 25},
		"pairs": {"QU": 6, "UU": -4, "?S": 2}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	return leaves
}

func TestLeaveValue(t *testing.T) {
	leaves := testLeaves(t)
	cases := map[string]float64{
		"":    0,
		"S":   8,
		"X":   0,
		"QU":  -7 - 3 + 6,
		"UQ":  -7 - 3 + 6, // order doesn't matter
		"UU":  -3 - 3 - 4,
		"UUU": -9 - 4*3, // three pairs
		"?S":  25 + 8 + 2,
	}
	for leave, want := range cases {
		if got := leaves.Value(leave); math.Abs(got-want) > 1e-9 {
			t.Errorf("value of %q = %v, want %v", leave, got, want)
		}
	}
}

func TestParseLeavesRejectsBadData(t *testing.T) {
	for _, text := range []string{`not json`, `{"singles":{"AB":1}}`, `{"singles":{"1":1}}`, `{"pairs":{"A":1}}`, `{"pairs":{"A1":1}}`} {
		if _, err := ParseLeaves([]byte(text)); err == nil {
			t.Errorf("expected an error for %s", text)
		}
	}
}

func across(row, col int, letters string) []game.Placement {
	var tiles []game.Placement
	for i := range len(letters) {
		tiles = append(tiles, game.Placement{Row: row, Col: col + i, Letter: letters[i]})
	}
	return tiles
}

func TestLanesOpened(t *testing.T) {
	// Triple words are at (0,3), (0,11), (3,0), (3,14), (11,0), (11,14), (14,3)
	// and (14,11).
	withTile := func(row, col int) *game.Board {
		var b game.Board
		b[row][col] = 'A'
		return &b
	}
	cases := []struct {
		name   string
		before *game.Board
		tiles  []game.Placement
		want   int
	}{
		{"away from every lane", &game.Board{}, across(7, 6, "AT"), 0},
		{"reaches triple words in both directions on column 3", &game.Board{}, across(7, 3, "A"), 2},
		{"one square too far from the top one", &game.Board{}, across(8, 3, "A"), 1},
		{"just out of reach of both", withTile(0, 0), across(7, 4, "A"), 0},
		{"lane already open", withTile(5, 3), across(4, 3, "A"), 0},
		// The old tile already opened the lane from (0,3), but not the one from (14,3).
		{"an old tile only covers its own lane", withTile(4, 3), across(7, 3, "A"), 1},
		{"covering the square opens nothing there", &game.Board{}, across(0, 3, "A"), 0},
		{"two tiles in the same lane count once", &game.Board{}, []game.Placement{{Row: 6, Col: 3, Letter: 'A'}, {Row: 5, Col: 3, Letter: 'A'}}, 1},
		{"opens a row lane", &game.Board{}, across(3, 7, "A"), 2}, // from (3,0) and (3,14)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LanesOpened(tc.before, tc.tiles); got != tc.want {
				t.Errorf("lanes = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestEvaluate(t *testing.T) {
	e := &Evaluator{Leaves: testLeaves(t), LanePenalty: 5}
	board := &game.Board{}
	// Plays A and T from column 3, rather than the centre: this test is only about
	// equity. (7,3) opens two lanes.
	move := game.ScorePlay(board, across(7, 3, "AT"))

	got, err := e.Evaluate(board, move, "ATQS", 20)
	if err != nil {
		t.Fatal(err)
	}
	if got.Leave != "QS" || got.LeaveValue != 1 || got.LanesOpened != 2 {
		t.Errorf("evaluation = %+v", got)
	}
	// AT on the double word: (1+1) x 2 = 4. Then +1 leave, -10 for two lanes.
	if got.Equity != -5 {
		t.Errorf("equity = %v, want -5", got.Equity)
	}
}

func TestEvaluateWithEmptyBagCountsTilesAgainstThePlayer(t *testing.T) {
	e := &Evaluator{Leaves: testLeaves(t)}
	board := &game.Board{}
	move := game.ScorePlay(board, across(7, 7, "AT"))

	got, err := e.Evaluate(board, move, "ATQS", 0)
	if err != nil {
		t.Fatal(err)
	}
	// Q and S cost 11. The learned value of 1 is ignored.
	if got.LeaveValue != -11 || got.Equity != 2-11 {
		t.Errorf("evaluation = %+v", got)
	}
}

func TestEvaluateRejectsTilesNotOnRack(t *testing.T) {
	e := &Evaluator{Leaves: testLeaves(t)}
	board := &game.Board{}
	move := game.ScorePlay(board, across(7, 7, "AT"))
	if _, err := e.Evaluate(board, move, "A", 10); err == nil {
		t.Error("expected an error")
	}
}
