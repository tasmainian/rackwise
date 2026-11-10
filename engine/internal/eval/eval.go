// Package eval estimates how good a play is beyond its score: the value of the
// tiles kept and the triple-word lanes it opens for the opponent.
//
// This is a static evaluation, with no simulation of future turns, so "best"
// here is an estimate. See docs/decisions/0001-own-engine.md.
package eval

import (
	"encoding/json"
	"fmt"

	"rackwise/engine/internal/game"
)

// DefaultLanePenalty is the equity lost per triple-word lane opened. Self-play
// found no clear benefit from it, though large values hurt; see
// docs/decisions/0005-leave-values.md.
const DefaultLanePenalty = 3.0

// laneReach is how far from a triple-word square a tile still gives the
// opponent access: a full rack played from that square reaches a tile 7 away.
const laneReach = game.RackSize

const tileKinds = 27 // A to Z, then the blank

// Leaves holds the learned values. A leave's value is the sum of its tiles'
// values plus an adjustment for each pair of tiles. That captures both duplicates
// (UU) and synergy (QU) with far fewer numbers than a table of every leave.
type Leaves struct {
	singles [tileKinds]float64
	pairs   [tileKinds][tileKinds]float64
}

// LeavesFile is the JSON form. Pair keys are two tiles in sorted order, like
// "?S", "EE" or "QU".
type LeavesFile struct {
	Singles map[string]float64 `json:"singles"`
	Pairs   map[string]float64 `json:"pairs"`
}

// ParseLeaves reads the JSON form.
func ParseLeaves(text []byte) (*Leaves, error) {
	var file LeavesFile
	if err := json.Unmarshal(text, &file); err != nil {
		return nil, fmt.Errorf("reading leave values: %w", err)
	}
	return file.Leaves()
}

// Leaves converts the JSON form to a lookup table.
func (f *LeavesFile) Leaves() (*Leaves, error) {
	leaves := &Leaves{}
	for key, value := range f.Singles {
		if len(key) != 1 || TileIndex(key[0]) < 0 {
			return nil, fmt.Errorf("bad single tile key %q", key)
		}
		leaves.singles[TileIndex(key[0])] = value
	}
	for key, value := range f.Pairs {
		if len(key) != 2 || TileIndex(key[0]) < 0 || TileIndex(key[1]) < 0 {
			return nil, fmt.Errorf("bad tile pair key %q", key)
		}
		a, b := TileIndex(key[0]), TileIndex(key[1])
		leaves.pairs[a][b] = value
		leaves.pairs[b][a] = value
	}
	return leaves, nil
}

// TileIndex maps a rack tile to 0-26, or -1 if it isn't one.
func TileIndex(tile byte) int {
	switch {
	case tile == game.Blank:
		return tileKinds - 1
	case tile >= 'A' && tile <= 'Z':
		return int(tile - 'A')
	}
	return -1
}

// Value returns how much the tiles are worth keeping, in points, compared with
// keeping nothing and drawing a full rack.
func (l *Leaves) Value(leave string) float64 {
	total := 0.0
	for i := range len(leave) {
		a := TileIndex(leave[i])
		total += l.singles[a]
		for j := i + 1; j < len(leave); j++ {
			total += l.pairs[a][TileIndex(leave[j])]
		}
	}
	return total
}

// Evaluator scores plays.
type Evaluator struct {
	Leaves      *Leaves
	LanePenalty float64
}

// Evaluation is everything known about a play beyond its score.
type Evaluation struct {
	Leave       string
	LeaveValue  float64
	LanesOpened int
	Equity      float64
}

// Evaluate works out a play's equity: score + leave value - opening penalty.
//
// With an empty bag no tiles can be drawn, so the learned values mean nothing.
// Each kept tile instead costs its points, because they count against the player
// at the end.
func (e *Evaluator) Evaluate(b *game.Board, move *game.Move, rack string, bagRemaining int) (Evaluation, error) {
	leave, err := game.RemoveTiles(rack, move.Tiles)
	if err != nil {
		return Evaluation{}, err
	}
	result := Evaluation{Leave: leave, LanesOpened: LanesOpened(b, move.Tiles)}
	if bagRemaining == 0 {
		result.LeaveValue = -float64(game.TilesValue(leave))
	} else {
		result.LeaveValue = e.Leaves.Value(leave)
	}
	result.Equity = float64(move.Score) + result.LeaveValue - e.LanePenalty*float64(result.LanesOpened)
	return result, nil
}

var tripleWordSquares = func() [][2]int {
	var squares [][2]int
	for r := range game.BoardSize {
		for c := range game.BoardSize {
			if game.PremiumAt(r, c) == game.TripleWord {
				squares = append(squares, [2]int{r, c})
			}
		}
	}
	return squares
}()

// LanesOpened counts the ways a play gives new access to an empty triple-word
// square. A lane is one direction out from such a square. It opens when the
// nearest tile within reach is one of the new tiles, and no older tile was
// already in reach that way, since then the lane was open before this play.
func LanesOpened(before *game.Board, tiles []game.Placement) int {
	after := *before
	after.Place(tiles)

	opened := 0
	for _, square := range tripleWordSquares {
		if after[square[0]][square[1]] != 0 {
			continue
		}
		for _, step := range [][2]int{{0, 1}, {0, -1}, {1, 0}, {-1, 0}} {
			if tileWithinReach(&after, square, step) && !tileWithinReach(before, square, step) {
				opened++
			}
		}
	}
	return opened
}

func tileWithinReach(b *game.Board, square, step [2]int) bool {
	for distance := 1; distance <= laneReach; distance++ {
		if b.Occupied(square[0]+step[0]*distance, square[1]+step[1]*distance) {
			return true
		}
	}
	return false
}
