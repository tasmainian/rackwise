package lexicon

import (
	"slices"
	"strings"
	"testing"

	"rackwise/engine/data"
)

func mustNew(t *testing.T, words ...string) *Lexicon {
	t.Helper()
	lex, err := New(words)
	if err != nil {
		t.Fatal(err)
	}
	return lex
}

// allWords lists the graph's contents by walking it, to prove nothing was lost
// or invented when suffixes were shared.
func allWords(l *Lexicon) []string {
	var words []string
	var visit func(Node, string)
	visit = func(n Node, prefix string) {
		if l.IsWord(n) {
			words = append(words, prefix)
		}
		for i := range 26 {
			if l.Letters(n)&(1<<i) != 0 {
				next, _ := l.Child(n, byte('A'+i))
				visit(next, prefix+string(rune('A'+i)))
			}
		}
	}
	visit(l.Root(), "")
	return words
}

func TestContains(t *testing.T) {
	lex := mustNew(t, "cat", "cats", "car", "dog", "cat")
	if lex.Count() != 4 {
		t.Errorf("count = %d, want 4", lex.Count())
	}
	for _, word := range []string{"CAT", "CATS", "CAR", "DOG"} {
		if !lex.Contains(word) {
			t.Errorf("%s should be present", word)
		}
	}
	for _, word := range []string{"CA", "CARS", "DO", "DOGS", "", "cat", "C1T", "ZZ"} {
		if lex.Contains(word) {
			t.Errorf("%q should be absent", word)
		}
	}
}

func TestSuffixesAreShared(t *testing.T) {
	// A plain trie needs a separate "ING" chain for each stem. With sharing, all
	// four stems end in the same nodes.
	lex := mustNew(t, "TAKING", "BAKING", "MAKING", "RAKING")
	// Root, four first letters, then shared A-K-I-N-G.
	if got := lex.NodeCount(); got != 7 {
		t.Errorf("nodes = %d, want 7", got)
	}
}

func TestUnplayableWordsAreDropped(t *testing.T) {
	lex := mustNew(t, "A", "AT", strings.Repeat("A", 15), strings.Repeat("A", 16))
	if lex.Count() != 2 {
		t.Errorf("count = %d, want 2", lex.Count())
	}
	if lex.Contains("A") || lex.Contains(strings.Repeat("A", 16)) {
		t.Error("words under 2 letters or over 15 should be dropped")
	}
}

func TestRejectsNonLetters(t *testing.T) {
	if _, err := New([]string{"it's"}); err == nil {
		t.Error("expected an error")
	}
}

func TestWalkAndLetters(t *testing.T) {
	lex := mustNew(t, "CAT", "CAR")
	n, ok := lex.Walk(lex.Root(), "CA")
	if !ok {
		t.Fatal("CA should be a path")
	}
	want := uint32(1<<('R'-'A') | 1<<('T'-'A'))
	if got := lex.Letters(n); got != want {
		t.Errorf("letters = %b, want %b", got, want)
	}
	if _, ok := lex.Walk(lex.Root(), "CX"); ok {
		t.Error("CX should not be a path")
	}
}

func TestEnableRoundTrips(t *testing.T) {
	lex, err := Parse(data.Enable)
	if err != nil {
		t.Fatal(err)
	}

	var want []string
	for _, word := range strings.Fields(string(data.Enable)) {
		if len(word) <= 15 {
			want = append(want, strings.ToUpper(word))
		}
	}
	slices.Sort(want)
	if got := allWords(lex); !slices.Equal(got, want) {
		t.Fatalf("graph holds %d words, list has %d playable words", len(got), len(want))
	}
	t.Logf("%d words in %d nodes", lex.Count(), lex.NodeCount())

	// Gaps in ENABLE itself. QI is added back by data.Dictionary.
	for _, word := range []string{"QI", "ZA"} {
		if lex.Contains(word) {
			t.Errorf("%s is not in ENABLE", word)
		}
	}
	for _, word := range []string{"QAT", "CWM", "AA", "RETAINS"} {
		if !lex.Contains(word) {
			t.Errorf("%s should be in ENABLE", word)
		}
	}
}

func TestDictionaryIsEnablePlusAdditions(t *testing.T) {
	enable, err := Parse(data.Enable)
	if err != nil {
		t.Fatal(err)
	}
	dictionary, err := Parse(data.Dictionary())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := dictionary.Count(), enable.Count()+1; got != want {
		t.Errorf("dictionary has %d words, want %d", got, want)
	}
	if !dictionary.Contains("QI") {
		t.Error("QI should be in the dictionary")
	}
	if dictionary.Contains("ZA") {
		t.Error("ZA should not be in the dictionary")
	}
}

func TestCommonWordsAreAllInEnable(t *testing.T) {
	lex, err := Parse(data.Enable)
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range strings.Fields(string(data.Common)) {
		if len(word) <= 15 && !lex.Contains(strings.ToUpper(word)) {
			t.Errorf("common word %q is not in ENABLE", word)
		}
	}
}
