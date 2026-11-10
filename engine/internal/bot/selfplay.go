package bot

import (
	"fmt"
	"math/rand/v2"

	"rackwise/engine/internal/game"
)

// Agent is one side in a self-play game.
type Agent struct {
	Bots  *Bots
	Level Level
}

// PlayGame plays a full game, going through the real game rules so that any
// illegal choice by a bot is an error. The same seed gives the same game.
func PlayGame(agents [2]Agent, seed uint64) (*game.Game, error) {
	g := game.NewGame(agents[0].Bots.Lex, rand.New(rand.NewPCG(seed, 0)))
	rngs := [2]*rand.Rand{rand.New(rand.NewPCG(seed, 1)), rand.New(rand.NewPCG(seed, 2))}

	for !g.Over {
		player := g.Current
		agent := agents[player]
		action, err := agent.Bots.Choose(agent.Level, &g.Board, g.Racks[player], g.Bag.Remaining(), rngs[player])
		if err != nil {
			return nil, err
		}
		if err := Apply(g, action); err != nil {
			return nil, fmt.Errorf("%s bot: %w", agent.Level, err)
		}
	}
	return g, nil
}

// Apply carries out a bot's decision through the real game rules, so an illegal
// choice is an error rather than a corrupted game.
func Apply(g *game.Game, action Action) error {
	switch action.Kind {
	case game.TurnPlay:
		if _, err := g.Play(action.Move.Tiles); err != nil {
			return fmt.Errorf("illegal play %s: %w", action.Move.Word, err)
		}
		return nil
	case game.TurnExchange:
		if err := g.Exchange(action.Exchange); err != nil {
			return fmt.Errorf("illegal exchange %s: %w", action.Exchange, err)
		}
		return nil
	}
	return g.Pass()
}
