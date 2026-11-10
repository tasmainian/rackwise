package session

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"rackwise/engine/internal/bot"
	"rackwise/engine/internal/game"
)

// MemoryStore keeps games in memory. It is used by tests and when the engine runs
// without a database, in which case games are lost on restart.
type MemoryStore struct {
	mu      sync.Mutex
	records map[string][]byte
}

// NewMemoryStore returns an empty store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{records: map[string][]byte{}}
}

// Insert implements Store.
func (m *MemoryStore) Insert(_ context.Context, record *Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.put(record)
}

// Records are stored encoded, so callers can't change a stored game by accident
// and the JSON form gets the same use as in Postgres.
func (m *MemoryStore) put(record *Record) error {
	text, err := json.Marshal(record)
	if err != nil {
		return err
	}
	m.records[record.ID] = text
	return nil
}

// Load implements Store.
func (m *MemoryStore) Load(_ context.Context, id string) (*Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	text, ok := m.records[id]
	if !ok {
		return nil, ErrNotFound
	}
	var record Record
	if err := json.Unmarshal(text, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

// Update implements Store.
func (m *MemoryStore) Update(_ context.Context, record *Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	text, ok := m.records[record.ID]
	if !ok {
		return ErrNotFound
	}
	var stored Record
	if err := json.Unmarshal(text, &stored); err != nil {
		return err
	}
	if stored.Version != record.Version {
		return ErrConflict
	}
	updated := *record
	updated.Version++
	return m.put(&updated)
}

//go:embed schema.sql
var schema string

// PostgresStore keeps games in Postgres.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// OpenPostgres connects and creates the tables if needed. Postgres only runs
// db/init on a brand-new volume, so applying the schema here also covers
// databases created by an earlier version.
func OpenPostgres(ctx context.Context, url string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connecting to the database: %w", err)
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("applying the schema: %w", err)
	}
	return &PostgresStore{pool: pool}, nil
}

// Close releases the connections.
func (p *PostgresStore) Close() { p.pool.Close() }

// savedState is the state column: everything needed to carry on with a game.
type savedState struct {
	Seed     uint64        `json:"seed"`
	Snapshot game.Snapshot `json:"snapshot"`
}

// Insert implements Store.
func (p *PostgresStore) Insert(ctx context.Context, record *Record) error {
	state, err := json.Marshal(savedState{record.Seed, record.State})
	if err != nil {
		return err
	}
	_, err = p.pool.Exec(ctx,
		`INSERT INTO games (id, bot_level, version, state) VALUES ($1, $2, $3, $4)`,
		record.ID, string(record.Level), record.Version, state)
	return err
}

// Load implements Store.
func (p *PostgresStore) Load(ctx context.Context, id string) (*Record, error) {
	var level string
	var version int
	var state []byte
	var record *string
	err := p.pool.QueryRow(ctx,
		`SELECT bot_level, version, state, gcg FROM games WHERE id = $1`, id,
	).Scan(&level, &version, &state, &record)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var saved savedState
	if err := json.Unmarshal(state, &saved); err != nil {
		return nil, fmt.Errorf("reading game %s: %w", id, err)
	}
	loaded := &Record{ID: id, Level: bot.Level(level), Seed: saved.Seed, Version: version, State: saved.Snapshot}
	if record != nil {
		loaded.GCG = *record
	}
	return loaded, nil
}

// Update implements Store. The version check stops a double-clicked submit, or a
// second open tab, from applying a turn to a game that has already moved on.
func (p *PostgresStore) Update(ctx context.Context, record *Record) error {
	state, err := json.Marshal(savedState{record.Seed, record.State})
	if err != nil {
		return err
	}
	var status string
	var finishedAt *time.Time
	var gcgText *string
	if record.State.Over {
		status = "finished"
		now := time.Now()
		finishedAt = &now
		gcgText = &record.GCG
	} else {
		status = "active"
	}
	tag, err := p.pool.Exec(ctx,
		`UPDATE games
		 SET version = version + 1, state = $3, status = $4, person_score = $5, bot_score = $6,
		     gcg = $7, finished_at = $8
		 WHERE id = $1 AND version = $2`,
		record.ID, record.Version, state, status,
		record.State.Scores[personSeat], record.State.Scores[botSeat], gcgText, finishedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err := p.Load(ctx, record.ID); err != nil {
			return err
		}
		return ErrConflict
	}
	return nil
}
