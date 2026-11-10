// Package gcg writes finished games in GCG, the standard game record format. A
// record keeps each player's rack on every turn, which is what the coach needs
// to review a move against the alternatives.
package gcg

import (
	"fmt"
	"strings"

	"rackwise/engine/internal/game"
)

// Info describes a game for the record's header.
type Info struct {
	ID string
	// Nicks are the names used on each turn line. GCG doesn't allow spaces in them.
	Nicks [2]string
	Names [2]string
}

// Write formats a finished game.
//
// Rackwise's end-of-game rule differs from the common "double the opponent's
// tiles" rule, so going out takes two lines: the player gains the opponent's tile
// values, and the opponent loses them. Both use standard GCG line forms.
func Write(info Info, g *game.Game) (string, error) {
	if !g.Over {
		return "", fmt.Errorf("game %s is not finished", info.ID)
	}

	var out strings.Builder
	out.WriteString("#character-encoding UTF-8\n")
	fmt.Fprintf(&out, "#player1 %s %s\n", info.Nicks[0], info.Names[0])
	fmt.Fprintf(&out, "#player2 %s %s\n", info.Nicks[1], info.Names[1])
	fmt.Fprintf(&out, "#id rackwise %s\n", info.ID)
	out.WriteString("#lexicon ENABLE\n")

	totals := [2]int{}
	for _, turn := range g.Turns {
		totals[turn.Player] += turn.Score
		nick, total := info.Nicks[turn.Player], totals[turn.Player]
		switch turn.Kind {
		case game.TurnPlay:
			fmt.Fprintf(&out, ">%s: %s %s %s +%d %d\n",
				nick, turn.Rack, turn.Move.Position(), playNotation(turn.Move), turn.Score, total)
		case game.TurnExchange:
			fmt.Fprintf(&out, ">%s: %s -%s +0 %d\n", nick, turn.Rack, turn.Exchanged, total)
		case game.TurnPass:
			fmt.Fprintf(&out, ">%s: %s - +0 %d\n", nick, turn.Rack, total)
		}
	}

	wentOut := -1
	for player := range 2 {
		if g.Racks[player] == "" {
			wentOut = player
		}
	}
	if wentOut >= 0 {
		// The player going out is first, so the running totals read naturally.
		opponent := 1 - wentOut
		rack := g.Racks[opponent]
		points := game.TilesValue(rack)
		totals[wentOut] += points
		fmt.Fprintf(&out, ">%s: (%s) +%d %d\n", info.Nicks[wentOut], rack, points, totals[wentOut])
		totals[opponent] -= points
		fmt.Fprintf(&out, ">%s: %s (%s) -%d %d\n", info.Nicks[opponent], rack, rack, points, totals[opponent])
	} else {
		for player := range 2 {
			rack := g.Racks[player]
			points := game.TilesValue(rack)
			totals[player] -= points
			fmt.Fprintf(&out, ">%s: %s (%s) -%d %d\n", info.Nicks[player], rack, rack, points, totals[player])
		}
	}

	if totals != g.Scores {
		return "", fmt.Errorf("game %s: record totals %v don't match final scores %v", info.ID, totals, g.Scores)
	}
	return out.String(), nil
}

// playNotation writes the main word with '.' for tiles already on the board.
func playNotation(move *game.Move) string {
	isNew := map[[2]int]bool{}
	for _, t := range move.Tiles {
		isNew[[2]int{t.Row, t.Col}] = true
	}
	notation := []byte(move.Word)
	for i := range notation {
		square := [2]int{move.Row, move.Col + i}
		if move.Dir == game.Down {
			square = [2]int{move.Row + i, move.Col}
		}
		if !isNew[square] {
			notation[i] = '.'
		}
	}
	return string(notation)
}
