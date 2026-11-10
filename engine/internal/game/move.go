package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Direction is the way a play's main word runs.
type Direction int

// MarshalText stores a direction by name, so saved games stay readable.
func (d Direction) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// UnmarshalText reads the form written by MarshalText.
func (d *Direction) UnmarshalText(text []byte) error {
	switch string(text) {
	case "across":
		*d = Across
	case "down":
		*d = Down
	default:
		return fmt.Errorf("unknown direction %q", text)
	}
	return nil
}

const (
	Across Direction = iota
	Down
)

func (d Direction) String() string {
	if d == Down {
		return "down"
	}
	return "across"
}

// Placement is one tile put on the board. Letter is lowercase for a blank.
type Placement struct {
	Row, Col int
	Letter   byte
}

type placementJSON struct {
	Row    int    `json:"row"`
	Col    int    `json:"col"`
	Letter string `json:"letter"`
}

// MarshalJSON stores the letter as text rather than a number.
func (p Placement) MarshalJSON() ([]byte, error) {
	return json.Marshal(placementJSON{p.Row, p.Col, string(p.Letter)})
}

// UnmarshalJSON reads the form written by MarshalJSON.
func (p *Placement) UnmarshalJSON(text []byte) error {
	var parsed placementJSON
	if err := json.Unmarshal(text, &parsed); err != nil {
		return err
	}
	if len(parsed.Letter) != 1 {
		return fmt.Errorf("tile letter %q must be one character", parsed.Letter)
	}
	*p = Placement{Row: parsed.Row, Col: parsed.Col, Letter: parsed.Letter[0]}
	return nil
}

// Move is a play with its words and score worked out.
type Move struct {
	Row int       `json:"row"` // where the main word starts
	Col int       `json:"col"`
	Dir Direction `json:"direction"`
	// Word is the whole main word, including tiles already on the board.
	// Lowercase letters are blanks.
	Word       string      `json:"word"`
	Tiles      []Placement `json:"tiles"`
	CrossWords []string    `json:"cross_words"`
	Score      int         `json:"score"`
	Bingo      bool        `json:"bingo"`
}

// Position returns the square in GCG notation: "8H" for across, "H8" for down.
func (m *Move) Position() string {
	col := string(rune('A' + m.Col))
	row := fmt.Sprint(m.Row + 1)
	if m.Dir == Down {
		return col + row
	}
	return row + col
}

// Words returns the main word followed by the cross-words.
func (m *Move) Words() []string {
	return append([]string{m.Word}, m.CrossWords...)
}

// Dictionary answers whether a word is valid. Words are passed in uppercase.
type Dictionary interface {
	Contains(word string) bool
}

// ErrorCode names why a play is illegal, for clients to act on.
type ErrorCode string

const (
	ErrNoTiles      ErrorCode = "no_tiles"
	ErrBadLetter    ErrorCode = "invalid_letter"
	ErrOutOfBounds  ErrorCode = "out_of_bounds"
	ErrOccupied     ErrorCode = "square_occupied"
	ErrDuplicate    ErrorCode = "duplicate_square"
	ErrNotInLine    ErrorCode = "not_in_line"
	ErrGap          ErrorCode = "gap_in_word"
	ErrNotOnRack    ErrorCode = "tile_not_on_rack"
	ErrNotCentered  ErrorCode = "must_cover_center"
	ErrNotConnected ErrorCode = "not_connected"
	ErrTooShort     ErrorCode = "word_too_short"
	ErrInvalidWord  ErrorCode = "invalid_word"
)

// PlayError explains why a play is illegal.
type PlayError struct {
	Code    ErrorCode
	Message string
}

func (e *PlayError) Error() string { return e.Message }

// CodeOf returns the code from a PlayError, or "" for any other error.
func CodeOf(err error) ErrorCode {
	var playErr *PlayError
	if errors.As(err, &playErr) {
		return playErr.Code
	}
	return ""
}

func playErrorf(code ErrorCode, format string, args ...any) *PlayError {
	return &PlayError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Validate checks every rule for a proposed play and returns it scored.
func Validate(b *Board, rack string, tiles []Placement, dict Dictionary) (*Move, error) {
	if err := checkShape(b, tiles); err != nil {
		return nil, err
	}
	if _, err := RemoveTiles(rack, tiles); err != nil {
		return nil, playErrorf(ErrNotOnRack, "%s", err)
	}

	move := ScorePlay(b, tiles)

	if b.IsEmpty() {
		if !coversCenter(tiles) {
			return nil, playErrorf(ErrNotCentered, "the first play must cover the centre square")
		}
		if len(tiles) < 2 {
			return nil, playErrorf(ErrTooShort, "words need at least 2 letters")
		}
	} else if !touchesExisting(b, tiles) {
		return nil, playErrorf(ErrNotConnected, "the play must connect to tiles on the board")
	}

	for _, word := range move.Words() {
		if !dict.Contains(strings.ToUpper(word)) {
			return nil, playErrorf(ErrInvalidWord, "%s is not a valid word", strings.ToUpper(word))
		}
	}
	return move, nil
}

// checkShape checks that the tiles sit on free squares in one unbroken line.
func checkShape(b *Board, tiles []Placement) error {
	if len(tiles) == 0 {
		return playErrorf(ErrNoTiles, "the play has no tiles")
	}
	seen := map[[2]int]bool{}
	sameRow, sameCol := true, true
	for _, t := range tiles {
		if !isLetter(t.Letter) {
			return playErrorf(ErrBadLetter, "%q is not a letter", t.Letter)
		}
		if !inBounds(t.Row, t.Col) {
			return playErrorf(ErrOutOfBounds, "row %d, column %d is off the board", t.Row, t.Col)
		}
		if b[t.Row][t.Col] != 0 {
			return playErrorf(ErrOccupied, "row %d, column %d already has a tile", t.Row, t.Col)
		}
		square := [2]int{t.Row, t.Col}
		if seen[square] {
			return playErrorf(ErrDuplicate, "two tiles were placed on row %d, column %d", t.Row, t.Col)
		}
		seen[square] = true
		sameRow = sameRow && t.Row == tiles[0].Row
		sameCol = sameCol && t.Col == tiles[0].Col
	}
	if !sameRow && !sameCol {
		return playErrorf(ErrNotInLine, "all tiles must be in one row or column")
	}

	// Every square between the first and last tile needs a tile, new or old.
	first, last := tiles[0], tiles[0]
	for _, t := range tiles {
		if t.Row < first.Row || t.Col < first.Col {
			first = t
		}
		if t.Row > last.Row || t.Col > last.Col {
			last = t
		}
	}
	dr, dc := 0, 1
	if !sameRow {
		dr, dc = 1, 0
	}
	for r, c := first.Row, first.Col; r <= last.Row && c <= last.Col; r, c = r+dr, c+dc {
		if b[r][c] == 0 && !seen[[2]int{r, c}] {
			return playErrorf(ErrGap, "there is a gap at row %d, column %d", r, c)
		}
	}
	return nil
}

func coversCenter(tiles []Placement) bool {
	return slices.ContainsFunc(tiles, func(t Placement) bool {
		return t.Row == Center && t.Col == Center
	})
}

func touchesExisting(b *Board, tiles []Placement) bool {
	for _, t := range tiles {
		if b.Occupied(t.Row-1, t.Col) || b.Occupied(t.Row+1, t.Col) ||
			b.Occupied(t.Row, t.Col-1) || b.Occupied(t.Row, t.Col+1) {
			return true
		}
	}
	return false
}

// ScorePlay works out a play's words and score. It does not check legality:
// the tiles must already form one unbroken line on free squares.
//
// Validation and move generation both use it, so there is one scoring routine.
func ScorePlay(b *Board, tiles []Placement) *Move {
	dir := playDirection(b, tiles)

	// Lay the tiles on a copy so that walking a word sees old and new together.
	after := *b
	after.Place(tiles)

	isNew := map[[2]int]bool{}
	for _, t := range tiles {
		isNew[[2]int{t.Row, t.Col}] = true
	}

	move := &Move{Dir: dir, Tiles: tiles, Bingo: len(tiles) == RackSize}
	var points int
	move.Row, move.Col, move.Word, points = scoreWord(&after, isNew, tiles[0].Row, tiles[0].Col, dir)
	move.Score = points

	crossDir := Down
	if dir == Down {
		crossDir = Across
	}
	for _, t := range tiles {
		_, _, word, crossPoints := scoreWord(&after, isNew, t.Row, t.Col, crossDir)
		if len(word) >= 2 {
			move.CrossWords = append(move.CrossWords, word)
			move.Score += crossPoints
		}
	}
	if move.Bingo {
		move.Score += BingoBonus
	}
	return move
}

// playDirection picks the main word's direction. A single tile can form a word
// each way, so it takes across when it has a neighbour in its row, and down
// otherwise. Move generation relies on this to list each single-tile play once.
func playDirection(b *Board, tiles []Placement) Direction {
	if len(tiles) > 1 {
		if tiles[0].Row == tiles[1].Row {
			return Across
		}
		return Down
	}
	t := tiles[0]
	if b.Occupied(t.Row, t.Col-1) || b.Occupied(t.Row, t.Col+1) {
		return Across
	}
	if b.Occupied(t.Row-1, t.Col) || b.Occupied(t.Row+1, t.Col) {
		return Down
	}
	return Across
}

// scoreWord finds the word through a square and scores it. Premiums count only
// on newly placed tiles: a premium is used up once a tile sits on it.
func scoreWord(after *Board, isNew map[[2]int]bool, row, col int, dir Direction) (startRow, startCol int, word string, points int) {
	dr, dc := 0, 1
	if dir == Down {
		dr, dc = 1, 0
	}
	for after.Occupied(row-dr, col-dc) {
		row, col = row-dr, col-dc
	}
	startRow, startCol = row, col

	var text strings.Builder
	sum, wordMultiplier := 0, 1
	for ; after.Occupied(row, col); row, col = row+dr, col+dc {
		letter := after[row][col]
		text.WriteByte(letter)
		value := TileValue(letter)
		if isNew[[2]int{row, col}] {
			switch PremiumAt(row, col) {
			case DoubleLetter:
				value *= 2
			case TripleLetter:
				value *= 3
			case DoubleWord:
				wordMultiplier *= 2
			case TripleWord:
				wordMultiplier *= 3
			}
		}
		sum += value
	}
	return startRow, startCol, text.String(), sum * wordMultiplier
}
