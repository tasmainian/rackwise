// Package session runs games between a person and a bot and keeps them in a store.
// The state is saved after every turn, so a game survives a page refresh or an
// engine restart. See docs/decisions/0006-game-sessions.md.
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	mathrand "math/rand/v2"

	"rackwise/engine/internal/bot"
	"rackwise/engine/internal/game"
	"rackwise/engine/internal/gcg"
)

// Seats. The person always moves first in this version.
const (
	personSeat = 0
	botSeat    = 1
)

// Errors a caller can act on.
var (
	ErrNotFound    = errors.New("game not found")
	ErrConflict    = errors.New("the game changed while this move was being made")
	ErrGameOver    = game.ErrGameOver
	ErrNotFinished = errors.New("the game isn't finished")
)

// Rejection means a move broke the rules. The game hasn't changed.
type Rejection struct {
	Code    string
	Message string
}

func (r *Rejection) Error() string { return r.Message }

// Record is one game as kept by a store. State holds the bot's rack and the bag,
// so it must never be sent to a player: View is the player's form.
type Record struct {
	ID    string
	Level bot.Level
	// Seed drives the shuffles. It would reveal future draws, so it stays private.
	Seed    uint64
	Version int
	State   game.Snapshot
	GCG     string
}

// Store keeps game records.
type Store interface {
	Insert(ctx context.Context, record *Record) error
	Load(ctx context.Context, id string) (*Record, error)
	// Update saves a record if the stored version still matches record.Version,
	// and otherwise returns ErrConflict. On success the stored version goes up by 1.
	Update(ctx context.Context, record *Record) error
}

// Service runs games.
type Service struct {
	bots  *bot.Bots
	store Store
}

// New returns a service.
func New(bots *bot.Bots, store Store) *Service {
	return &Service{bots: bots, store: store}
}

// Create starts a game. A nil seed picks a random one.
func (s *Service) Create(ctx context.Context, level bot.Level, seed *uint64) (*View, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	record := &Record{ID: hex.EncodeToString(id), Level: level, Seed: mathrand.Uint64()}
	if seed != nil {
		record.Seed = *seed
	}
	record.State = game.NewGame(s.bots.Lex, mathrand.New(mathrand.NewPCG(record.Seed, 0))).Snapshot()
	if err := s.store.Insert(ctx, record); err != nil {
		return nil, err
	}
	return newView(record), nil
}

// Get returns a game as the person may see it.
func (s *Service) Get(ctx context.Context, id string) (*View, error) {
	record, err := s.store.Load(ctx, id)
	if err != nil {
		return nil, err
	}
	return newView(record), nil
}

// Play makes the person's play, then the bot's reply.
func (s *Service) Play(ctx context.Context, id string, tiles []game.Placement) (*View, error) {
	return s.takeTurn(ctx, id, func(g *game.Game) error {
		_, err := g.Play(tiles)
		var playErr *game.PlayError
		if errors.As(err, &playErr) {
			return &Rejection{Code: string(playErr.Code), Message: playErr.Message}
		}
		return err
	})
}

// Exchange swaps the person's tiles, then the bot replies.
func (s *Service) Exchange(ctx context.Context, id, tiles string) (*View, error) {
	return s.takeTurn(ctx, id, func(g *game.Game) error {
		err := g.Exchange(tiles)
		if err != nil && !errors.Is(err, game.ErrGameOver) {
			return &Rejection{Code: "cannot_exchange", Message: err.Error()}
		}
		return err
	})
}

// Pass skips the person's turn, then the bot replies.
func (s *Service) Pass(ctx context.Context, id string) (*View, error) {
	return s.takeTurn(ctx, id, func(g *game.Game) error { return g.Pass() })
}

// GCG returns a finished game's record.
func (s *Service) GCG(ctx context.Context, id string) (string, error) {
	record, err := s.store.Load(ctx, id)
	if err != nil {
		return "", err
	}
	if !record.State.Over {
		return "", ErrNotFinished
	}
	return record.GCG, nil
}

// takeTurn applies the person's action and the bot's reply together. Nothing is
// saved unless both succeed, so a stored game is never left waiting on the bot.
func (s *Service) takeTurn(ctx context.Context, id string, act func(*game.Game) error) (*View, error) {
	record, err := s.store.Load(ctx, id)
	if err != nil {
		return nil, err
	}
	if record.State.Over {
		return nil, ErrGameOver
	}

	// Mixing in the turn count keeps the random stream different after each reload,
	// while the same seed and moves still replay the same game.
	stream := uint64(len(record.State.Turns))
	g, err := game.Restore(record.State, s.bots.Lex, mathrand.New(mathrand.NewPCG(record.Seed, 2*stream+1)))
	if err != nil {
		return nil, err
	}
	if err := act(g); err != nil {
		return nil, err
	}
	if !g.Over {
		botRNG := mathrand.New(mathrand.NewPCG(record.Seed, 2*stream+2))
		action, err := s.bots.Choose(record.Level, &g.Board, g.Racks[botSeat], g.Bag.Remaining(), botRNG)
		if err != nil {
			return nil, err
		}
		if err := bot.Apply(g, action); err != nil {
			return nil, fmt.Errorf("%s bot: %w", record.Level, err)
		}
	}

	record.State = g.Snapshot()
	if g.Over {
		record.GCG, err = gcg.Write(gcgInfo(record), g)
		if err != nil {
			return nil, err
		}
	}
	if err := s.store.Update(ctx, record); err != nil {
		return nil, err
	}
	record.Version++
	return newView(record), nil
}

func gcgInfo(record *Record) gcg.Info {
	return gcg.Info{
		ID:    record.ID,
		Nicks: [2]string{"Player", "Rackwise_" + string(record.Level)},
		Names: [2]string{"Player", "Rackwise " + string(record.Level) + " bot"},
	}
}
