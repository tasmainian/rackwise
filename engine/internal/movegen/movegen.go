// Package movegen finds every legal play for a rack, using the algorithm from
// Appel and Jacobson, "The World's Fastest Scrabble Program" (1988).
//
// It only looks for across plays. Down plays are found by running the same code
// on the transposed board. Because Appel-Jacobson walks words left to right from
// the start of the word, each play is built exactly once.
package movegen

import (
	"cmp"
	"slices"

	"rackwise/engine/internal/game"
	"rackwise/engine/internal/lexicon"
)

const allLetters = uint32(1<<26) - 1

// Generate returns every legal play, highest score first. The order is fully
// deterministic, so equal-scoring plays always appear in the same order.
func Generate(b *game.Board, rack string, lex *lexicon.Lexicon) []*game.Move {
	moves := generatePass(b, b, rack, lex, false)
	moves = append(moves, generatePass(b, b.Transpose(), rack, lex, true)...)
	Sort(moves)
	return moves
}

// Sort orders moves by score, then by a stable description of the play.
func Sort(moves []*game.Move) {
	slices.SortFunc(moves, func(x, y *game.Move) int {
		return cmp.Or(
			cmp.Compare(y.Score, x.Score),
			cmp.Compare(len(x.Tiles), len(y.Tiles)),
			cmp.Compare(x.Word, y.Word),
			cmp.Compare(x.Row, y.Row),
			cmp.Compare(x.Col, y.Col),
			cmp.Compare(x.Dir, y.Dir),
		)
	})
}

// pass holds the state for one direction.
type pass struct {
	original *game.Board // used for scoring
	board    *game.Board // the board being searched, transposed for down plays
	lex      *lexicon.Lexicon
	flipped  bool

	crossChecks [game.BoardSize][game.BoardSize]uint32
	counts      [26]int
	blanks      int

	row    int
	anchor int
	// prefix holds tiles placed left of the anchor, in word order.
	prefix []byte
	// placed holds tiles placed from the anchor rightwards.
	placed []game.Placement
	moves  []*game.Move
}

func generatePass(original, searched *game.Board, rack string, lex *lexicon.Lexicon, flipped bool) []*game.Move {
	p := &pass{original: original, board: searched, lex: lex, flipped: flipped}
	for i := range len(rack) {
		if rack[i] == game.Blank {
			p.blanks++
		} else {
			p.counts[rack[i]-'A']++
		}
	}
	p.computeCrossChecks()

	for row := range game.BoardSize {
		p.row = row
		for col := range game.BoardSize {
			if p.isAnchor(row, col) {
				p.searchAnchor(col)
			}
		}
	}
	return p.moves
}

// isAnchor reports whether a square can start the search for a play. Every play
// must cover at least one anchor: an empty square next to a tile, or the centre
// on an empty board.
func (p *pass) isAnchor(row, col int) bool {
	if p.board[row][col] != 0 {
		return false
	}
	if p.board.IsEmpty() {
		return row == game.Center && col == game.Center
	}
	return p.board.Occupied(row-1, col) || p.board.Occupied(row+1, col) ||
		p.board.Occupied(row, col-1) || p.board.Occupied(row, col+1)
}

// computeCrossChecks precomputes, for each empty square, which letters form a
// valid word with the tiles above and below it. Search then only tests a bit.
func (p *pass) computeCrossChecks() {
	for row := range game.BoardSize {
		for col := range game.BoardSize {
			p.crossChecks[row][col] = allLetters
			if p.board[row][col] == 0 && (p.board.Occupied(row-1, col) || p.board.Occupied(row+1, col)) {
				p.crossChecks[row][col] = p.crossCheck(row, col)
			}
		}
	}
}

func (p *pass) crossCheck(row, col int) uint32 {
	start := row
	for p.board.Occupied(start-1, col) {
		start--
	}
	above := make([]byte, 0, row-start)
	for r := start; r < row; r++ {
		above = append(above, upper(p.board[r][col]))
	}
	var below []byte
	for r := row + 1; p.board.Occupied(r, col); r++ {
		below = append(below, upper(p.board[r][col]))
	}

	node, ok := p.lex.Walk(p.lex.Root(), string(above))
	if !ok {
		return 0
	}
	var mask uint32
	for letter := byte('A'); letter <= 'Z'; letter++ {
		child, ok := p.lex.Child(node, letter)
		if !ok {
			continue
		}
		if end, ok := p.lex.Walk(child, string(below)); ok && p.lex.IsWord(end) {
			mask |= 1 << (letter - 'A')
		}
	}
	return mask
}

// searchAnchor finds plays whose leftmost anchor is this square. Tiles to its
// left are either already on the board, or are placed only over squares that
// aren't anchors. That rule is what stops a play being found twice.
func (p *pass) searchAnchor(anchor int) {
	p.anchor = anchor
	p.prefix = p.prefix[:0]
	p.placed = p.placed[:0]

	if p.board.Occupied(p.row, anchor-1) {
		start := anchor - 1
		for p.board.Occupied(p.row, start-1) {
			start--
		}
		node := p.lex.Root()
		for col := start; col < anchor; col++ {
			next, ok := p.lex.Child(node, upper(p.board[p.row][col]))
			if !ok {
				return
			}
			node = next
		}
		p.extendRight(node, anchor)
		return
	}

	limit := 0
	for col := anchor - 1; col >= 0 && p.board[p.row][col] == 0 && !p.isAnchor(p.row, col); col-- {
		limit++
	}
	p.leftPart(p.lex.Root(), limit)
}

// leftPart tries every prefix up to limit tiles, then extends right from the
// anchor. Prefix squares aren't anchors, so nothing sits above or below them
// and they need no cross-check.
func (p *pass) leftPart(node lexicon.Node, limit int) {
	p.extendRight(node, p.anchor)
	if limit == 0 {
		return
	}
	p.forEachTile(node, allLetters, func(next lexicon.Node, played byte) {
		p.prefix = append(p.prefix, played)
		p.leftPart(next, limit-1)
		p.prefix = p.prefix[:len(p.prefix)-1]
	})
}

func (p *pass) extendRight(node lexicon.Node, col int) {
	if col >= game.BoardSize || p.board[p.row][col] == 0 {
		// A word only counts if it has reached the anchor and stopped at a free
		// square or the edge.
		if col > p.anchor && p.lex.IsWord(node) {
			p.record()
		}
		if col >= game.BoardSize {
			return
		}
		p.forEachTile(node, p.crossChecks[p.row][col], func(next lexicon.Node, played byte) {
			p.placed = append(p.placed, game.Placement{Row: p.row, Col: col, Letter: played})
			p.extendRight(next, col+1)
			p.placed = p.placed[:len(p.placed)-1]
		})
		return
	}
	if next, ok := p.lex.Child(node, upper(p.board[p.row][col])); ok {
		p.extendRight(next, col+1)
	}
}

// forEachTile tries each allowed letter that continues a word, using a real tile
// and, separately, a blank. Those are different plays with different scores.
func (p *pass) forEachTile(node lexicon.Node, allowed uint32, try func(lexicon.Node, byte)) {
	options := p.lex.Letters(node) & allowed
	for i := range 26 {
		if options&(1<<i) == 0 {
			continue
		}
		next, _ := p.lex.Child(node, byte('A'+i))
		if p.counts[i] > 0 {
			p.counts[i]--
			try(next, byte('A'+i))
			p.counts[i]++
		}
		if p.blanks > 0 {
			p.blanks--
			try(next, byte('a'+i))
			p.blanks++
		}
	}
}

// record turns the current search state into a scored move on the real board.
func (p *pass) record() {
	tiles := make([]game.Placement, 0, len(p.prefix)+len(p.placed))
	col := p.anchor - len(p.prefix)
	for i, letter := range p.prefix {
		tiles = append(tiles, p.placement(p.row, col+i, letter))
	}
	for _, t := range p.placed {
		tiles = append(tiles, p.placement(t.Row, t.Col, t.Letter))
	}

	// A single tile can form words both ways, and would be found by both
	// passes. It belongs to the across pass when it has a row neighbour, which
	// matches the direction game.ScorePlay gives it.
	if p.flipped && len(tiles) == 1 {
		t := tiles[0]
		if p.original.Occupied(t.Row, t.Col-1) || p.original.Occupied(t.Row, t.Col+1) {
			return
		}
	}
	p.moves = append(p.moves, game.ScorePlay(p.original, tiles))
}

func (p *pass) placement(row, col int, letter byte) game.Placement {
	if p.flipped {
		row, col = col, row
	}
	return game.Placement{Row: row, Col: col, Letter: letter}
}

func upper(letter byte) byte {
	if letter >= 'a' {
		return letter - ('a' - 'A')
	}
	return letter
}
