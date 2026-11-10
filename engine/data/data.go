// Package data embeds the word lists and leave values, so the engine binary has
// no files to find at runtime.
package data

import (
	_ "embed"
	"slices"
)

// Enable is the unmodified ENABLE word list. Use Dictionary to play the game.
//
//go:embed enable1.txt
var Enable []byte

// Additions is the words Rackwise accepts beyond ENABLE. They are kept apart so
// that enable1.txt stays byte-identical to its source. See
// docs/decisions/0007-word-additions.md.
//
//go:embed additions.txt
var Additions []byte

// Dictionary returns the game's word list: ENABLE plus Additions.
func Dictionary() []byte {
	return slices.Concat(Enable, []byte("\n"), Additions)
}

// Common is the words the easy bot may play. cmd/gencommon builds it.
//
//go:embed common.txt
var Common []byte

// Leaves holds the leave values. cmd/genleaves builds it.
//
//go:embed leaves.json
var Leaves []byte
