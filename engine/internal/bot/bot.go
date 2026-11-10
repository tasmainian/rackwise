// Package bot chooses turns for the robot opponent at three strengths.
package bot

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"

	"rackwise/engine/data"
	"rackwise/engine/internal/eval"
	"rackwise/engine/internal/game"
	"rackwise/engine/internal/lexicon"
	"rackwise/engine/internal/movegen"
)

// Level is a bot's strength.
type Level string

const (
	Easy   Level = "easy"
	Medium Level = "medium"
	Hard   Level = "hard"
)

// ParseLevel checks a level name.
func ParseLevel(name string) (Level, error) {
	switch level := Level(name); level {
	case Easy, Medium, Hard:
		return level, nil
	}
	return "", fmt.Errorf("unknown bot level %q: use easy, medium or hard", name)
}

// DefaultExchangeBelow is the equity under which the hard bot would rather
// exchange than play. Self-play found values from 0 to 10 equally good; see
// docs/decisions/0005-leave-values.md.
const DefaultExchangeBelow = 5.0

// easyDecentShare is how close to its best option a move must be for the easy
// bot to consider it, so it plays plausibly rather than purely at random.
const easyDecentShare = 0.5

// easyCheckedLength is the shortest word the easy bot needs to know. Common word
// lists hold few 2-letter words, so requiring them would block most overlaps.
const easyCheckedLength = 3

// Bots holds what the bots need to decide. It is safe for concurrent use.
type Bots struct {
	Lex           *lexicon.Lexicon
	Common        *lexicon.Lexicon
	Eval          *eval.Evaluator
	ExchangeBelow float64
}

// Default returns bots using the embedded data, building them once.
var Default = sync.OnceValues(func() (*Bots, error) {
	lex, err := lexicon.Parse(data.Dictionary())
	if err != nil {
		return nil, fmt.Errorf("loading dictionary: %w", err)
	}
	common, err := lexicon.Parse(data.Common)
	if err != nil {
		return nil, fmt.Errorf("loading common words: %w", err)
	}
	leaves, err := eval.ParseLeaves(data.Leaves)
	if err != nil {
		return nil, err
	}
	return &Bots{
		Lex:           lex,
		Common:        common,
		Eval:          &eval.Evaluator{Leaves: leaves, LanePenalty: eval.DefaultLanePenalty},
		ExchangeBelow: DefaultExchangeBelow,
	}, nil
})

// Action is a bot's decision for its turn.
type Action struct {
	Kind game.TurnKind
	Move *game.Move // for a play
	// Evaluation describes Move, and is only set for a play.
	Evaluation eval.Evaluation
	Exchange   string // for an exchange
}

// Choose picks a turn. Only the easy bot uses rng.
func (b *Bots) Choose(level Level, board *game.Board, rack string, bagRemaining int, rng *rand.Rand) (Action, error) {
	moves := movegen.Generate(board, rack, b.Lex)
	var action Action
	var err error
	switch level {
	case Easy:
		action = b.chooseEasy(moves, rack, bagRemaining, rng)
	case Medium:
		action = chooseMedium(moves, rack, bagRemaining)
	case Hard:
		action, err = b.chooseHard(board, moves, rack, bagRemaining)
	default:
		err = fmt.Errorf("unknown bot level %q", level)
	}
	if err != nil {
		return Action{}, err
	}
	// The hard bot has already evaluated its play in order to choose it.
	if action.Kind == game.TurnPlay && level != Hard {
		action.Evaluation, err = b.Eval.Evaluate(board, action.Move, rack, bagRemaining)
	}
	return action, err
}

// chooseEasy picks at random among moves that use only familiar words and score
// at least half of the best such move.
func (b *Bots) chooseEasy(moves []*game.Move, rack string, bagRemaining int, rng *rand.Rand) Action {
	var familiar []*game.Move
	for _, move := range moves {
		if b.usesFamiliarWords(move) {
			familiar = append(familiar, move)
		}
	}
	if len(familiar) == 0 {
		return exchangeOrPass(rack, bagRemaining)
	}
	// Moves are sorted by score, so the decent ones come first.
	floor := float64(familiar[0].Score) * easyDecentShare
	decent := 0
	for decent < len(familiar) && float64(familiar[decent].Score) >= floor {
		decent++
	}
	return Action{Kind: game.TurnPlay, Move: familiar[rng.IntN(decent)]}
}

func (b *Bots) usesFamiliarWords(move *game.Move) bool {
	for _, word := range move.Words() {
		if len(word) >= easyCheckedLength && !b.Common.Contains(strings.ToUpper(word)) {
			return false
		}
	}
	return true
}

func chooseMedium(moves []*game.Move, rack string, bagRemaining int) Action {
	if len(moves) == 0 {
		return exchangeOrPass(rack, bagRemaining)
	}
	return Action{Kind: game.TurnPlay, Move: moves[0]}
}

// chooseHard takes the play with the highest equity, or exchanges if even that
// is poor and an exchange is allowed.
func (b *Bots) chooseHard(board *game.Board, moves []*game.Move, rack string, bagRemaining int) (Action, error) {
	var best Action
	haveBest := false
	for _, move := range moves {
		evaluation, err := b.Eval.Evaluate(board, move, rack, bagRemaining)
		if err != nil {
			return Action{}, err
		}
		if !haveBest || evaluation.Equity > best.Evaluation.Equity {
			best = Action{Kind: game.TurnPlay, Move: move, Evaluation: evaluation}
			haveBest = true
		}
	}
	canExchange := bagRemaining >= game.MinBagForExchange
	if canExchange && (!haveBest || best.Evaluation.Equity < b.ExchangeBelow) {
		return Action{Kind: game.TurnExchange, Exchange: b.bestExchange(rack)}, nil
	}
	if !haveBest {
		return Action{Kind: game.TurnPass}, nil
	}
	return best, nil
}

// bestExchange returns the tiles to give up: the ones not in the most valuable
// set that can be kept, always giving up at least one.
func (b *Bots) bestExchange(rack string) string {
	var bestGiveUp string
	bestValue := 0.0
	// Each bit of keep says whether to keep that tile. The all-ones set is left
	// out, since that exchanges nothing.
	for keep := range 1<<len(rack) - 1 {
		var kept, giveUp []byte
		for i := range len(rack) {
			if keep&(1<<i) != 0 {
				kept = append(kept, rack[i])
			} else {
				giveUp = append(giveUp, rack[i])
			}
		}
		if value := b.Eval.Leaves.Value(string(kept)); bestGiveUp == "" || value > bestValue {
			bestGiveUp, bestValue = string(giveUp), value
		}
	}
	return bestGiveUp
}

func exchangeOrPass(rack string, bagRemaining int) Action {
	if bagRemaining >= game.MinBagForExchange && rack != "" {
		return Action{Kind: game.TurnExchange, Exchange: rack}
	}
	return Action{Kind: game.TurnPass}
}
