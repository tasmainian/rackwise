package game

import (
	"errors"
	"fmt"
	"math/rand/v2"
)

// Bag holds the undrawn tiles in random order.
type Bag struct {
	tiles []byte
	rng   *rand.Rand
}

// NewBag returns a shuffled full tile set.
func NewBag(rng *rand.Rand) *Bag {
	bag := &Bag{tiles: FullTileSet(), rng: rng}
	bag.shuffle()
	return bag
}

func (b *Bag) shuffle() {
	b.rng.Shuffle(len(b.tiles), func(i, j int) {
		b.tiles[i], b.tiles[j] = b.tiles[j], b.tiles[i]
	})
}

// Remaining returns how many tiles are left.
func (b *Bag) Remaining() int { return len(b.tiles) }

// Draw takes up to n tiles.
func (b *Bag) Draw(n int) string {
	n = min(n, len(b.tiles))
	drawn := string(b.tiles[:n])
	b.tiles = b.tiles[n:]
	return drawn
}

// Return puts tiles back and reshuffles.
func (b *Bag) Return(tiles string) {
	b.tiles = append(b.tiles, tiles...)
	b.shuffle()
}

// TurnKind is what a player did on their turn.
type TurnKind string

const (
	TurnPlay     TurnKind = "play"
	TurnExchange TurnKind = "exchange"
	TurnPass     TurnKind = "pass"
)

// Turn records one turn. Rack is kept because the coach and GCG records need
// what the player held, not just what they played.
type Turn struct {
	Player    int      `json:"player"`
	Kind      TurnKind `json:"kind"`
	Rack      string   `json:"rack"`                // before the turn
	Move      *Move    `json:"move,omitempty"`      // for a play
	Exchanged string   `json:"exchanged,omitempty"` // for an exchange
	Score     int      `json:"score"`
	BagAfter  int      `json:"bag_after"` // tiles left in the bag once the turn finished
}

// Game is a two-player game in progress.
type Game struct {
	Board  Board
	Bag    *Bag
	Racks  [2]string
	Scores [2]int
	// Current is the player whose turn it is.
	Current int
	Turns   []Turn
	Over    bool
	// EndAdjustments holds the points each player gained or lost at the end,
	// already included in Scores.
	EndAdjustments [2]int

	dict      Dictionary
	scoreless int
}

// ErrGameOver is returned for any action after the game has ended.
var ErrGameOver = errors.New("the game is over")

// NewGame deals a game. The same random source gives the same game.
func NewGame(dict Dictionary, rng *rand.Rand) *Game {
	g := &Game{Bag: NewBag(rng), dict: dict}
	g.Racks[0] = sortTiles(g.Bag.Draw(RackSize))
	g.Racks[1] = sortTiles(g.Bag.Draw(RackSize))
	return g
}

// Play makes a play for the current player.
func (g *Game) Play(tiles []Placement) (*Move, error) {
	if g.Over {
		return nil, ErrGameOver
	}
	rack := g.Racks[g.Current]
	move, err := Validate(&g.Board, rack, tiles, g.dict)
	if err != nil {
		return nil, err
	}
	remaining, err := RemoveTiles(rack, tiles)
	if err != nil {
		return nil, err
	}

	g.Board.Place(tiles)
	g.Scores[g.Current] += move.Score
	g.Racks[g.Current] = sortTiles(remaining + g.Bag.Draw(len(tiles)))
	g.Turns = append(g.Turns, Turn{Player: g.Current, Kind: TurnPlay, Rack: rack, Move: move, Score: move.Score, BagAfter: g.Bag.Remaining()})

	// A play worth nothing (only possible with blanks) counts as scoreless.
	if move.Score == 0 {
		g.scoreless++
	} else {
		g.scoreless = 0
	}

	if g.Racks[g.Current] == "" {
		g.finishWentOut()
		return move, nil
	}
	g.afterTurn()
	return move, nil
}

// Exchange swaps tiles for the current player. New tiles are drawn before the
// old ones go back, so a player can't redraw what they just gave up.
func (g *Game) Exchange(tiles string) error {
	if g.Over {
		return ErrGameOver
	}
	if tiles == "" {
		return errors.New("no tiles to exchange")
	}
	if g.Bag.Remaining() < MinBagForExchange {
		return fmt.Errorf("exchanging needs at least %d tiles in the bag, there are %d", MinBagForExchange, g.Bag.Remaining())
	}
	rack := g.Racks[g.Current]
	remaining, err := removeFromRack(rack, []byte(tiles))
	if err != nil {
		return err
	}
	g.Racks[g.Current] = sortTiles(remaining + g.Bag.Draw(len(tiles)))
	g.Bag.Return(tiles)
	g.Turns = append(g.Turns, Turn{Player: g.Current, Kind: TurnExchange, Rack: rack, Exchanged: sortTiles(tiles), BagAfter: g.Bag.Remaining()})
	g.scoreless++
	g.afterTurn()
	return nil
}

// Pass skips the current player's turn.
func (g *Game) Pass() error {
	if g.Over {
		return ErrGameOver
	}
	g.Turns = append(g.Turns, Turn{Player: g.Current, Kind: TurnPass, Rack: g.Racks[g.Current], BagAfter: g.Bag.Remaining()})
	g.scoreless++
	g.afterTurn()
	return nil
}

func (g *Game) afterTurn() {
	if g.scoreless >= MaxScorelessTurns {
		g.finishScoreless()
		return
	}
	g.Current = 1 - g.Current
}

// finishWentOut applies end penalties when the current player used their last
// tile: the opponent loses their remaining tile values, and the player who went
// out gains them.
func (g *Game) finishWentOut() {
	opponent := 1 - g.Current
	remainder := TilesValue(g.Racks[opponent])
	g.adjust(opponent, -remainder)
	g.adjust(g.Current, remainder)
	g.Over = true
}

// finishScoreless applies end penalties after a run of scoreless turns: each
// player loses their own remaining tile values.
func (g *Game) finishScoreless() {
	for player := range 2 {
		g.adjust(player, -TilesValue(g.Racks[player]))
	}
	g.Over = true
}

// Snapshot is a game's full state in a form that can be saved. It holds the
// bot's rack and the bag order, so it must never be sent to a player.
type Snapshot struct {
	Board          []string  `json:"board"`
	Bag            string    `json:"bag"`
	Racks          [2]string `json:"racks"`
	Scores         [2]int    `json:"scores"`
	Current        int       `json:"current"`
	Turns          []Turn    `json:"turns"`
	Over           bool      `json:"over"`
	EndAdjustments [2]int    `json:"end_adjustments"`
	Scoreless      int       `json:"scoreless"`
}

// Snapshot captures the game's state.
func (g *Game) Snapshot() Snapshot {
	return Snapshot{
		Board:          g.Board.Rows(),
		Bag:            string(g.Bag.tiles),
		Racks:          g.Racks,
		Scores:         g.Scores,
		Current:        g.Current,
		Turns:          g.Turns,
		Over:           g.Over,
		EndAdjustments: g.EndAdjustments,
		Scoreless:      g.scoreless,
	}
}

// Restore rebuilds a game from a snapshot. rng drives any later reshuffles.
func Restore(s Snapshot, dict Dictionary, rng *rand.Rand) (*Game, error) {
	board, err := ParseBoard(s.Board)
	if err != nil {
		return nil, fmt.Errorf("restoring board: %w", err)
	}
	return &Game{
		Board:          *board,
		Bag:            &Bag{tiles: []byte(s.Bag), rng: rng},
		Racks:          s.Racks,
		Scores:         s.Scores,
		Current:        s.Current,
		Turns:          s.Turns,
		Over:           s.Over,
		EndAdjustments: s.EndAdjustments,
		dict:           dict,
		scoreless:      s.Scoreless,
	}, nil
}

// RemoveExchanged returns a rack without exchanged tiles. A plain tile list,
// unlike RemoveTiles' placements, is what an exchange records.
func RemoveExchanged(rack, tiles string) (string, error) {
	return removeFromRack(rack, []byte(tiles))
}

func (g *Game) adjust(player, points int) {
	g.Scores[player] += points
	g.EndAdjustments[player] += points
}
