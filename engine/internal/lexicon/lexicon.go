// Package lexicon stores a word list as a DAWG (directed acyclic word graph): a
// trie in which identical suffixes are shared, so ENABLE fits in little memory.
// It only needs to walk forward letter by letter, which is what Appel-Jacobson
// move generation does. See docs/decisions/0004-dawg-movegen.md.
package lexicon

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/bits"
	"slices"
	"strings"

	"rackwise/engine/internal/game"
)

// Node identifies a position in the graph.
type Node uint32

// node is one state. The children of a node sit together in Lexicon.edges,
// ordered by letter. mask has a bit per letter that has a child, so a child's
// offset is the number of set bits below its own: no per-node array of 26.
type node struct {
	mask     uint32
	first    uint32
	terminal bool
}

// Lexicon is an immutable word list, safe for concurrent use.
type Lexicon struct {
	nodes []node
	edges []Node
	root  Node
	count int
}

// Parse reads newline-separated words.
func Parse(text []byte) (*Lexicon, error) {
	var words []string
	for line := range bytes.Lines(text) {
		if word := strings.TrimSpace(string(line)); word != "" {
			words = append(words, word)
		}
	}
	return New(words)
}

// New builds a lexicon. Words are uppercased and deduplicated. Words that could
// never be played, under 2 letters or longer than the board, are dropped.
func New(words []string) (*Lexicon, error) {
	cleaned := make([]string, 0, len(words))
	for _, word := range words {
		word = strings.ToUpper(word)
		for i := range len(word) {
			if word[i] < 'A' || word[i] > 'Z' {
				return nil, fmt.Errorf("word %q has a character outside A-Z", word)
			}
		}
		if len(word) >= 2 && len(word) <= game.BoardSize {
			cleaned = append(cleaned, word)
		}
	}
	slices.Sort(cleaned)
	cleaned = slices.Compact(cleaned)

	b := newBuilder()
	for _, word := range cleaned {
		b.add(word)
	}
	lex := b.finish()
	lex.count = len(cleaned)
	return lex, nil
}

// Root is where every word starts.
func (l *Lexicon) Root() Node { return l.root }

// Count returns how many words are stored.
func (l *Lexicon) Count() int { return l.count }

// NodeCount returns the graph's size, used to show how much sharing saved.
func (l *Lexicon) NodeCount() int { return len(l.nodes) }

// Child follows a letter (A-Z).
func (l *Lexicon) Child(n Node, letter byte) (Node, bool) {
	bit := uint32(1) << (letter - 'A')
	state := &l.nodes[n]
	if state.mask&bit == 0 {
		return 0, false
	}
	return l.edges[state.first+uint32(bits.OnesCount32(state.mask&(bit-1)))], true
}

// Letters returns a bitmask of the letters that can follow, with bit 0 for A.
func (l *Lexicon) Letters(n Node) uint32 { return l.nodes[n].mask }

// IsWord reports whether the path to this node spells a word.
func (l *Lexicon) IsWord(n Node) bool { return l.nodes[n].terminal }

// Walk follows a string from n. Letters must be uppercase.
func (l *Lexicon) Walk(n Node, letters string) (Node, bool) {
	for i := range len(letters) {
		next, ok := l.Child(n, letters[i])
		if !ok {
			return 0, false
		}
		n = next
	}
	return n, true
}

// Contains reports whether an uppercase word is in the list.
func (l *Lexicon) Contains(word string) bool {
	for i := range len(word) {
		if word[i] < 'A' || word[i] > 'Z' {
			return false
		}
	}
	n, ok := l.Walk(l.root, word)
	return ok && l.IsWord(n)
}

// builder is Daciuk's incremental algorithm for a minimal DAWG. Words arrive
// sorted, so once a word diverges from the previous one, the previous word's
// tail can never change. That tail is frozen, reusing an identical frozen node
// if one exists.
type builder struct {
	// register finds a frozen node by a signature of its contents.
	register map[string]uint32
	nodes    []builderNode
	// path holds the current word's nodes, which can still gain children.
	path     []*builderNode
	previous string
}

type builderNode struct {
	terminal bool
	letters  []byte
	targets  []uint32
}

func newBuilder() *builder {
	return &builder{register: map[string]uint32{}, path: []*builderNode{{}}}
}

func (b *builder) add(word string) {
	common := 0
	for common < len(word) && common < len(b.previous) && word[common] == b.previous[common] {
		common++
	}
	b.freezeDownTo(common)
	for i := common; i < len(word); i++ {
		// The letter is recorded now, but the edge's target is only added when
		// the child freezes and has a final index.
		b.path[i].letters = append(b.path[i].letters, word[i])
		b.path = append(b.path, &builderNode{})
	}
	b.path[len(word)].terminal = true
	b.previous = word
}

func (b *builder) freezeDownTo(depth int) {
	for len(b.path)-1 > depth {
		last := b.path[len(b.path)-1]
		b.path = b.path[:len(b.path)-1]
		parent := b.path[len(b.path)-1]
		parent.targets = append(parent.targets, b.freeze(last))
	}
}

func (b *builder) freeze(n *builderNode) uint32 {
	key := signature(n)
	if index, ok := b.register[key]; ok {
		return index
	}
	index := uint32(len(b.nodes))
	b.nodes = append(b.nodes, *n)
	b.register[key] = index
	return index
}

func signature(n *builderNode) string {
	buf := make([]byte, 0, 1+len(n.letters)*5)
	if n.terminal {
		buf = append(buf, 1)
	} else {
		buf = append(buf, 0)
	}
	for i, letter := range n.letters {
		buf = append(buf, letter)
		buf = binary.LittleEndian.AppendUint32(buf, n.targets[i])
	}
	return string(buf)
}

func (b *builder) finish() *Lexicon {
	b.freezeDownTo(0)
	root := b.freeze(b.path[0])

	lex := &Lexicon{nodes: make([]node, len(b.nodes)), root: Node(root)}
	for i := range b.nodes {
		n := &b.nodes[i]
		lex.nodes[i] = node{first: uint32(len(lex.edges)), terminal: n.terminal}
		for j, letter := range n.letters {
			lex.nodes[i].mask |= 1 << (letter - 'A')
			lex.edges = append(lex.edges, Node(n.targets[j]))
		}
	}
	return lex
}
