-- Runs once, when the database volume is first created.
-- pgvector stores the strategy note and mistake embeddings (Phases 5 and 6).
CREATE EXTENSION IF NOT EXISTS vector;
