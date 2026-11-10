package gcg

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"rackwise/engine/internal/bot"
	"rackwise/engine/internal/game"
)

type anyWord struct{}

func (anyWord) Contains(string) bool { return true }

var testInfo = Info{
	ID:    "g1",
	Nicks: [2]string{"Player", "Rackwise_Hard"},
	Names: [2]string{"You", "Rackwise Hard Bot"},
}

func across(row, col int, letters string) []game.Placement {
	var tiles []game.Placement
	for i := range len(letters) {
		if letters[i] != '.' {
			tiles = append(tiles, game.Placement{Row: row, Col: col + i, Letter: letters[i]})
		}
	}
	return tiles
}

func snapshotGame(t *testing.T, racks [2]string, bag string) *game.Game {
	t.Helper()
	g, err := game.Restore(game.Snapshot{
		Board: (&game.Board{}).Rows(),
		Bag:   bag,
		Racks: racks,
	}, anyWord{}, rand.New(rand.NewPCG(1, 1)))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestGoingOutRecord(t *testing.T) {
	g := snapshotGame(t, [2]string{"?CTX", "AEQ"}, "")
	if _, err := g.Play(across(7, 6, "CaT")); err != nil {
		t.Fatal(err)
	}
	if err := g.Pass(); err != nil {
		t.Fatal(err)
	}
	// X hooks the end, so the record shows the old tiles as dots. anyWord accepts it.
	if _, err := g.Play(across(7, 9, "X")); err != nil {
		t.Fatal(err)
	}
	if !g.Over {
		t.Fatalf("expected the game to end: %+v", g.Racks)
	}

	got, err := Write(testInfo, g)
	if err != nil {
		t.Fatal(err)
	}
	// CaT: C3 + a0 + T1 = 4. Then ...X: C3 + a0 + T1 + X8 = 12, for a total of 16.
	// A, E and Q are worth 12, gained by the player and lost by the opponent.
	want := `#character-encoding UTF-8
#player1 Player You
#player2 Rackwise_Hard Rackwise Hard Bot
#id rackwise g1
#lexicon ENABLE
>Player: ?CTX 8G CaT +4 4
>Rackwise_Hard: AEQ - +0 0
>Player: X 8G ...X +12 16
>Player: (AEQ) +12 28
>Rackwise_Hard: AEQ (AEQ) -12 -12
`
	if got != want {
		t.Errorf("record:\n%s\nwant:\n%s", got, want)
	}
}

func TestDownPlayAndExchangeNotation(t *testing.T) {
	g := snapshotGame(t, [2]string{"ATQQQQQ", "BCDEFGH"}, "IIIIIIIIII")
	if err := g.Exchange("QQ"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Play([]game.Placement{{Row: 7, Col: 7, Letter: 'B'}, {Row: 8, Col: 7, Letter: 'E'}}); err != nil {
		t.Fatal(err)
	}
	for range 6 {
		if g.Over {
			break
		}
		if err := g.Pass(); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Write(testInfo, g)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		">Player: ATQQQQQ -QQ +0 0\n",
		">Rackwise_Hard: BCDEFGH H8 BE +5 5\n",
	} {
		if !strings.Contains(got, line) {
			t.Errorf("missing %q in:\n%s", line, got)
		}
	}
}

func TestScorelessEndRecord(t *testing.T) {
	g := snapshotGame(t, [2]string{"AB", "QZ"}, "")
	for range game.MaxScorelessTurns {
		if err := g.Pass(); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Write(testInfo, g)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, ">Player: AB (AB) -5 -5\n>Rackwise_Hard: QZ (QZ) -19 -19\n") {
		t.Errorf("record:\n%s", got)
	}
}

func TestRefusesUnfinishedGame(t *testing.T) {
	g := snapshotGame(t, [2]string{"AB", "QZ"}, "")
	if _, err := Write(testInfo, g); err == nil {
		t.Error("expected an error")
	}
}

var (
	playLine     = regexp.MustCompile(`^>(\S+): (\S+) (\d+[A-O]|[A-O]\d+) (\S+) \+(\d+) (-?\d+)$`)
	exchangeLine = regexp.MustCompile(`^>(\S+): (\S+) -([A-Z?]+) \+0 (-?\d+)$`)
	passLine     = regexp.MustCompile(`^>(\S+): (\S+) - \+0 (-?\d+)$`)
	gainLine     = regexp.MustCompile(`^>(\S+): \(([A-Z?]*)\) \+(\d+) (-?\d+)$`)
	lossLine     = regexp.MustCompile(`^>(\S+): ([A-Z?]+) \(([A-Z?]+)\) -(\d+) (-?\d+)$`)
)

// replay rebuilds a game from its record alone. If the record has an error in a
// position, word or rack, the game rules reject it or the scores stop matching.
func replay(record string, nicks [2]string, dict game.Dictionary) ([2]int, error) {
	seat := map[string]int{nicks[0]: 0, nicks[1]: 1}
	var board game.Board
	totals := [2]int{}
	for _, line := range strings.Split(strings.TrimSpace(record), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if m := playLine.FindStringSubmatch(line); m != nil {
			player, rack := seat[m[1]], m[2]
			tiles, err := parsePlay(m[3], m[4], &board)
			if err != nil {
				return totals, fmt.Errorf("%s: %w", line, err)
			}
			move, err := game.Validate(&board, rack, tiles, dict)
			if err != nil {
				return totals, fmt.Errorf("%s: %w", line, err)
			}
			if score, _ := strconv.Atoi(m[5]); score != move.Score {
				return totals, fmt.Errorf("%s: rules score it %d", line, move.Score)
			}
			board.Place(tiles)
			totals[player] += move.Score
			if total, _ := strconv.Atoi(m[6]); total != totals[player] {
				return totals, fmt.Errorf("%s: running total should be %d", line, totals[player])
			}
			continue
		}
		var total, change int
		var player int
		switch {
		case exchangeLine.MatchString(line):
			m := exchangeLine.FindStringSubmatch(line)
			player = seat[m[1]]
			total, _ = strconv.Atoi(m[4])
		case passLine.MatchString(line):
			m := passLine.FindStringSubmatch(line)
			player = seat[m[1]]
			total, _ = strconv.Atoi(m[3])
		case gainLine.MatchString(line):
			m := gainLine.FindStringSubmatch(line)
			player = seat[m[1]]
			change, _ = strconv.Atoi(m[3])
			total, _ = strconv.Atoi(m[4])
			if change != game.TilesValue(m[2]) {
				return totals, fmt.Errorf("%s: tiles are worth %d", line, game.TilesValue(m[2]))
			}
		case lossLine.MatchString(line):
			m := lossLine.FindStringSubmatch(line)
			player = seat[m[1]]
			points, _ := strconv.Atoi(m[4])
			change = -points
			total, _ = strconv.Atoi(m[5])
		default:
			return totals, fmt.Errorf("unrecognised line %q", line)
		}
		totals[player] += change
		if total != totals[player] {
			return totals, fmt.Errorf("%s: running total should be %d", line, totals[player])
		}
	}
	return totals, nil
}

func parsePlay(position, word string, board *game.Board) ([]game.Placement, error) {
	var row, col, dr, dc int
	if position[0] >= 'A' && position[0] <= 'O' {
		number, err := strconv.Atoi(position[1:])
		if err != nil {
			return nil, err
		}
		col, row, dr = int(position[0]-'A'), number-1, 1
	} else {
		number, err := strconv.Atoi(position[:len(position)-1])
		if err != nil {
			return nil, err
		}
		col, row, dc = int(position[len(position)-1]-'A'), number-1, 1
	}
	var tiles []game.Placement
	for i := range len(word) {
		r, c := row+dr*i, col+dc*i
		if word[i] == '.' {
			if !board.Occupied(r, c) {
				return nil, fmt.Errorf("'.' at an empty square %d,%d", r, c)
			}
			continue
		}
		tiles = append(tiles, game.Placement{Row: r, Col: c, Letter: word[i]})
	}
	return tiles, nil
}

func TestRecordsOfRealGamesReplayExactly(t *testing.T) {
	bots, err := bot.Default()
	if err != nil {
		t.Fatal(err)
	}
	for seed := range uint64(10) {
		g, err := bot.PlayGame([2]bot.Agent{{Bots: bots, Level: bot.Medium}, {Bots: bots, Level: bot.Hard}}, seed)
		if err != nil {
			t.Fatal(err)
		}
		record, err := Write(testInfo, g)
		if err != nil {
			t.Fatal(err)
		}
		totals, err := replay(record, testInfo.Nicks, bots.Lex)
		if err != nil {
			t.Fatalf("seed %d: %v\n%s", seed, err, record)
		}
		if totals != g.Scores {
			t.Errorf("seed %d: replayed %v, final %v", seed, totals, g.Scores)
		}
	}
}
