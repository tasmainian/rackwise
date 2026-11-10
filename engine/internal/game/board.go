package game

import (
	"fmt"
	"slices"
	"strings"
)

// Board is the grid of played tiles. Zero means an empty square, an uppercase
// letter is a normal tile, and a lowercase letter is a blank standing for it.
type Board [BoardSize][BoardSize]byte

// ParseBoard reads the text form: 15 rows of 15 characters, using '.' for empty.
func ParseBoard(rows []string) (*Board, error) {
	if len(rows) != BoardSize {
		return nil, fmt.Errorf("board has %d rows, want %d", len(rows), BoardSize)
	}
	var board Board
	for r, row := range rows {
		if len(row) != BoardSize {
			return nil, fmt.Errorf("board row %d has %d squares, want %d", r, len(row), BoardSize)
		}
		for c := range BoardSize {
			ch := row[c]
			switch {
			case ch == '.':
			case isLetter(ch):
				board[r][c] = ch
			default:
				return nil, fmt.Errorf("board row %d column %d has invalid character %q", r, c, ch)
			}
		}
	}
	return &board, nil
}

// Rows returns the text form accepted by ParseBoard.
func (b *Board) Rows() []string {
	rows := make([]string, BoardSize)
	for r := range BoardSize {
		var row strings.Builder
		for c := range BoardSize {
			if b[r][c] == 0 {
				row.WriteByte('.')
			} else {
				row.WriteByte(b[r][c])
			}
		}
		rows[r] = row.String()
	}
	return rows
}

// IsEmpty reports whether no tile has been played.
func (b *Board) IsEmpty() bool {
	return *b == Board{}
}

// Transpose swaps rows and columns. Move generation only handles across plays,
// so it runs on the transposed board to find down plays.
func (b *Board) Transpose() *Board {
	var t Board
	for r := range BoardSize {
		for c := range BoardSize {
			t[c][r] = b[r][c]
		}
	}
	return &t
}

// Occupied reports whether a square is on the board and holds a tile.
func (b *Board) Occupied(row, col int) bool {
	return inBounds(row, col) && b[row][col] != 0
}

// Place writes tiles to the board without any checks.
func (b *Board) Place(tiles []Placement) {
	for _, t := range tiles {
		b[t.Row][t.Col] = t.Letter
	}
}

func inBounds(row, col int) bool {
	return row >= 0 && row < BoardSize && col >= 0 && col < BoardSize
}

func isLetter(ch byte) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

// ParseRack checks a rack's characters and size and returns it sorted.
func ParseRack(rack string) (string, error) {
	if len(rack) > RackSize {
		return "", fmt.Errorf("rack has %d tiles, maximum is %d", len(rack), RackSize)
	}
	for i := range len(rack) {
		if ch := rack[i]; ch != Blank && (ch < 'A' || ch > 'Z') {
			return "", fmt.Errorf("rack has invalid tile %q: use A-Z, or ? for a blank", ch)
		}
	}
	return sortTiles(rack), nil
}

// RemoveTiles returns the rack without the tiles a play uses, sorted. A blank
// placement (lowercase letter) uses a '?'. It fails if the rack lacks a tile.
func RemoveTiles(rack string, tiles []Placement) (string, error) {
	played := make([]byte, len(tiles))
	for i, t := range tiles {
		played[i] = rackTile(t.Letter)
	}
	return removeFromRack(rack, played)
}

func removeFromRack(rack string, tiles []byte) (string, error) {
	remaining := []byte(rack)
	for _, tile := range tiles {
		i := slices.Index(remaining, tile)
		if i < 0 {
			return "", fmt.Errorf("tile %q is not on the rack", tile)
		}
		remaining = slices.Delete(remaining, i, i+1)
	}
	return sortTiles(string(remaining)), nil
}

func rackTile(letter byte) byte {
	if letter >= 'a' && letter <= 'z' {
		return Blank
	}
	return letter
}

func sortTiles(tiles string) string {
	sorted := []byte(tiles)
	slices.Sort(sorted)
	return string(sorted)
}

// TilesValue sums the points of some tiles.
func TilesValue(tiles string) int {
	total := 0
	for i := range len(tiles) {
		total += TileValue(tiles[i])
	}
	return total
}
