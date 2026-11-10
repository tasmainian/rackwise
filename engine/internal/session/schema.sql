-- Applied at engine startup, so every statement must be safe to repeat.
CREATE TABLE IF NOT EXISTS games (
    id           TEXT PRIMARY KEY,
    bot_level    TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'active',
    -- Goes up by 1 on each save, so two saves can't both apply to the same turn.
    version      INTEGER NOT NULL DEFAULT 0,
    -- Full game state, including the bot's rack and bag order. Never sent to players.
    state        JSONB NOT NULL,
    -- Scores are also columns, so stats queries don't have to parse the state.
    person_score INTEGER NOT NULL DEFAULT 0,
    bot_score    INTEGER NOT NULL DEFAULT 0,
    gcg          TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS games_finished_at ON games (finished_at) WHERE finished_at IS NOT NULL;
