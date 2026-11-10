// Package game holds Rackwise's rules: the board, tiles, scoring, move
// validation and the turn-by-turn game state. It knows nothing about how moves
// are found or chosen.
package game

// Rackwise deliberately uses its own premium layout and tile set rather than
// copying an existing game's. See docs/decisions/0003-original-rules.md.
const (
	BoardSize = 15
	RackSize  = 7
	// Center is both the row and the column of the start square.
	Center = BoardSize / 2

	BingoBonus        = 40
	MinBagForExchange = 7
	// MaxScorelessTurns is the run of scoreless turns, across both players,
	// that ends the game.
	MaxScorelessTurns = 6

	// Blank is how an unplayed blank appears on a rack. Once played, a blank is
	// stored as the lowercase form of the letter it stands for.
	Blank = '?'
)

// Premium is the bonus on a square.
type Premium byte

const (
	NoPremium    Premium = '.'
	DoubleLetter Premium = 'd'
	TripleLetter Premium = 't'
	DoubleWord   Premium = 'D'
	TripleWord   Premium = 'T'
)

// premiumQuadrant is the upper-left quadrant, including the centre row and
// column. Premium mirrors it to cover the whole board. The start square has no
// premium.
var premiumQuadrant = [Center + 1]string{
	"t..T..d.",
	".D...t..",
	"..d.D..d",
	"T..t...D",
	"..D..d..",
	".t..d.t.",
	"d....t..",
	"..dD....",
}

// PremiumAt returns the bonus on a square.
func PremiumAt(row, col int) Premium {
	return Premium(premiumQuadrant[mirror(row)][mirror(col)])
}

func mirror(i int) int {
	return min(i, BoardSize-1-i)
}

// letterValues holds the points for A to Z.
var letterValues = [26]int{
	1, 4, 3, 2, 1, 4, 3, 4, 1, 9, 6, 1, 3, // A to M
	1, 1, 3, 10, 1, 1, 1, 2, 6, 5, 8, 4, 9, // N to Z
}

// letterCounts holds how many of A to Z are in a full tile set.
var letterCounts = [26]int{
	9, 2, 3, 4, 11, 2, 3, 2, 8, 1, 1, 5, 3, // A to M
	6, 7, 2, 1, 6, 5, 6, 4, 1, 2, 1, 2, 1, // N to Z
}

const blankCount = 2

// TileValue returns a tile's points. Blanks, played or not, are worth nothing.
func TileValue(tile byte) int {
	if tile >= 'A' && tile <= 'Z' {
		return letterValues[tile-'A']
	}
	return 0
}

// FullTileSet returns every tile in the game, in a fixed order.
func FullTileSet() []byte {
	tiles := make([]byte, 0, 100)
	for i, count := range letterCounts {
		for range count {
			tiles = append(tiles, byte('A'+i))
		}
	}
	for range blankCount {
		tiles = append(tiles, Blank)
	}
	return tiles
}
