"""Strategy notes: parsing, embedding and retrieval for the coach.

Retrieval filters by tags that plain code computes for a turn, then ranks the matching notes
by embedding similarity. Claude only sees each note's ID, title and summary.
See docs/decisions/0010-strategy-retrieval.md.

    python -m rackwise_coach.notes --load    # embed changed notes into Postgres
"""

import argparse
import hashlib
import logging
import os
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Protocol
from urllib.parse import unquote, urlparse

import numpy as np

from rackwise_coach import analysis

logger = logging.getLogger(__name__)
# A static embedding model: numpy only, no torch or onnxruntime, and small enough to ship
# in the coach image. The tags do the precise filtering, so this only ranks a few notes.
DEFAULT_EMBEDDING_MODEL = "minishlab/potion-base-8M"
NOTES_PER_TURN = 3
# These match facts computed by analysis.mistake_tags. A note may only use these.
TAGS = frozenset(
    {
        "duplicates",
        "vowel_heavy",
        "consonant_heavy",
        "q_without_u",
        "blank_kept",
        "blank_spent",
        "s_spent",
        "missed_bingo",
        "premium_missed",
        "low_score",
        "swap",
        "invalid_play",
    }
)
_DEFAULT_NOTES = Path(__file__).resolve().parents[3] / "notes"
_ID = re.compile(r"N\d{2}")
# Claude may quote a note's title or summary, and check_coaching rejects capitalised words
# and numbers that aren't in the game data, so those two fields must contain neither.
_UNQUOTABLE = re.compile(r"\d|\b[A-Z]{2,}\b")


class NoteError(Exception):
    """A note file is malformed."""


@dataclass(frozen=True)
class Note:
    id: str
    title: str
    tags: tuple[str, ...]
    summary: str
    body: str

    def text(self) -> str:
        """What gets embedded."""
        return f"{self.title}. {self.summary} {self.body}"

    def brief(self) -> dict:
        """What Claude sees. The body stays out of the prompt to keep it small."""
        return {"id": self.id, "title": self.title, "summary": self.summary}


def load_notes(directory: Path | None = None) -> list[Note]:
    directory = directory or Path(os.environ.get("NOTES_DIR", _DEFAULT_NOTES))
    notes = [parse_note(path.read_text(), path.name) for path in sorted(directory.glob("N*.md"))]
    ids = [note.id for note in notes]
    if len(set(ids)) != len(ids):
        raise NoteError("two notes share an ID")
    return notes


def parse_note(text: str, name: str = "note") -> Note:
    match = re.fullmatch(r"---\n(.*?)\n---\n(.*)", text, re.DOTALL)
    if not match:
        raise NoteError(f"{name}: missing the --- header")
    fields = {}
    for line in match.group(1).splitlines():
        key, _, value = line.partition(":")
        fields[key.strip()] = value.strip()
    missing = {"id", "title", "tags", "summary"} - fields.keys()
    if missing:
        raise NoteError(f"{name}: missing {', '.join(sorted(missing))}")
    tags = tuple(tag.strip() for tag in fields["tags"].split(",") if tag.strip())
    note = Note(fields["id"], fields["title"], tags, fields["summary"], match.group(2).strip())
    if not _ID.fullmatch(note.id):
        raise NoteError(f"{name}: the ID must look like N01")
    if not tags or set(tags) - TAGS:
        raise NoteError(f"{name}: unknown or missing tags {sorted(set(tags) - TAGS)}")
    if _UNQUOTABLE.search(note.title) or _UNQUOTABLE.search(note.summary):
        raise NoteError(f"{name}: the title and summary can't contain digits or capitalised words")
    if not note.body:
        raise NoteError(f"{name}: the body is empty")
    return note


class Embedder:
    """Turns text into vectors with a local model, loaded on first use."""

    def __init__(self, model_name: str | None = None):
        self.model_name = model_name or os.environ.get("EMBEDDING_MODEL", DEFAULT_EMBEDDING_MODEL)
        self._model = None

    def embed(self, texts: list[str]) -> np.ndarray:
        if self._model is None:
            from model2vec import StaticModel  # Imported here so tests don't load it.

            self._model = StaticModel.from_pretrained(self.model_name)
        return self._model.encode(texts)


class NoteStore(Protocol):
    def search(self, tags: list[str] | None, vector: np.ndarray, k: int) -> list[dict]:
        """The k nearest notes, only from notes sharing a tag when `tags` is given."""
        ...


class MemoryStore:
    """Holds the notes in memory. Used by tests and evals, which shouldn't need Postgres."""

    def __init__(self, notes: list[Note], embedder: Embedder):
        self._notes = notes
        self._vectors = _normalise(embedder.embed([note.text() for note in notes]))

    def search(self, tags: list[str] | None, vector: np.ndarray, k: int) -> list[dict]:
        wanted = set(tags or [])
        distances = 1 - self._vectors @ _normalise(vector[None, :])[0]
        ranked = sorted(zip(distances, self._notes, strict=True), key=lambda pair: pair[0])
        matching = [note for _, note in ranked if not wanted or wanted & set(note.tags)]
        return [note.brief() for note in matching[:k]]


class PostgresStore:
    """Stores the notes and their embeddings in pgvector."""

    def __init__(self, conn):
        from pgvector.pg8000 import register_vector

        self._conn = conn
        register_vector(conn)

    def sync(self, notes: list[Note], embedder: Embedder) -> int:
        """Embed the notes that changed, drop removed ones, and return how many were embedded."""
        # The column has no fixed dimension, so changing EMBEDDING_MODEL needs no migration.
        # Fifty rows don't need an index, so a sequential scan is fine.
        self._conn.run(
            "CREATE TABLE IF NOT EXISTS strategy_notes ("
            "id text PRIMARY KEY, title text NOT NULL, tags text[] NOT NULL,"
            " summary text NOT NULL, body text NOT NULL, digest text NOT NULL,"
            " embedding vector NOT NULL)"
        )
        stored = dict(self._conn.run("SELECT id, digest FROM strategy_notes"))
        digests = {note.id: _digest(note, embedder.model_name) for note in notes}
        changed = [note for note in notes if stored.get(note.id) != digests[note.id]]
        vectors = embedder.embed([note.text() for note in changed]) if changed else []
        for note, vector in zip(changed, vectors, strict=True):
            self._conn.run(
                "INSERT INTO strategy_notes (id, title, tags, summary, body, digest, embedding)"
                " VALUES (:id, :title, CAST(:tags AS text[]), :summary, :body, :digest, :vec)"
                " ON CONFLICT (id) DO UPDATE SET title = EXCLUDED.title, tags = EXCLUDED.tags,"
                " summary = EXCLUDED.summary, body = EXCLUDED.body, digest = EXCLUDED.digest,"
                " embedding = EXCLUDED.embedding",
                id=note.id,
                title=note.title,
                tags=list(note.tags),
                summary=note.summary,
                body=note.body,
                digest=digests[note.id],
                vec=np.asarray(vector),
            )
        removed = set(stored) - set(digests)
        if removed:
            self._conn.run("DELETE FROM strategy_notes WHERE id = ANY(:ids)", ids=list(removed))
        return len(changed)

    def search(self, tags: list[str] | None, vector: np.ndarray, k: int) -> list[dict]:
        where = "WHERE tags && CAST(:tags AS text[])" if tags else ""
        params = {"tags": tags} if tags else {}
        rows = self._conn.run(
            f"SELECT id, title, summary FROM strategy_notes {where}"
            " ORDER BY embedding <=> :vec LIMIT :k",
            vec=np.asarray(vector),
            k=k,
            **params,
        )
        return [{"id": row[0], "title": row[1], "summary": row[2]} for row in rows]

    def bodies(self, ids: list[str]) -> dict[str, dict]:
        """The full notes to show the player, keyed by ID."""
        if not ids:
            return {}
        rows = self._conn.run(
            "SELECT id, title, body FROM strategy_notes WHERE id = ANY(:ids)", ids=ids
        )
        return {row[0]: {"title": row[1], "body": row[2]} for row in rows}


def retrieve(
    store: NoteStore, embedder: Embedder, queries: dict[str, str], k: int = NOTES_PER_TURN
) -> list[dict]:
    """Tags first, then embeddings: for each tag in order, the note closest to its query.

    `queries` maps each tag to a description of that problem. Taking one note per tag keeps
    a broad tag from crowding out a specific one. If no note has any of the tags, the notes
    closest to the first query are used.
    """
    vectors = embedder.embed(list(queries.values()))
    chosen: dict[str, dict] = {}
    for tag, vector in zip(queries, vectors, strict=True):
        fresh = [note for note in store.search([tag], vector, k) if note["id"] not in chosen]
        if fresh:
            chosen[fresh[0]["id"]] = fresh[0]
        if len(chosen) == k:
            break
    return list(chosen.values()) or store.search(None, vectors[0], k)


def notes_for_turns(
    turns: list[dict], store: NoteStore, embedder: Embedder, k: int = NOTES_PER_TURN
) -> dict[int, list[dict]]:
    """Notes for each turn that plain code marks as a mistake. Other turns get none."""
    found = {}
    for turn in turns:
        tags = analysis.mistake_tags(turn)
        if tags:
            queries = {tag: analysis.mistake_query(turn, tag) for tag in tags}
            found[turn["turn"]] = retrieve(store, embedder, queries, k)
    return found


def connect(database_url: str | None = None):
    """Open a pg8000 connection from a postgresql:// URL."""
    import pg8000.native

    url = urlparse(database_url or os.environ["DATABASE_URL"])
    return pg8000.native.Connection(
        user=unquote(url.username or ""),
        password=unquote(url.password or ""),
        host=url.hostname or "localhost",
        port=url.port or 5432,
        database=url.path.lstrip("/"),
        timeout=10,
    )


def _digest(note: Note, model_name: str) -> str:
    """Changes when the note or the model changes, so only those notes are embedded again."""
    content = "\n".join([model_name, note.text(), ",".join(note.tags)])
    return hashlib.sha256(content.encode()).hexdigest()


def _normalise(vectors: np.ndarray) -> np.ndarray:
    return vectors / np.linalg.norm(vectors, axis=1, keepdims=True)


def main() -> None:
    logging.basicConfig(level=logging.INFO, format="%(message)s")
    parser = argparse.ArgumentParser(description="Embed the strategy notes into Postgres.")
    parser.add_argument("--load", action="store_true", help="embed changed notes into Postgres")
    parser.add_argument("--download", action="store_true", help="only fetch the embedding model")
    args = parser.parse_args()
    embedder = Embedder()
    if args.download:
        embedder.embed(["warm up"])
        print(f"Downloaded {embedder.model_name}")
        return
    notes = load_notes()
    print(f"{len(notes)} notes are valid")
    if args.load:
        conn = connect()
        try:
            embedded = PostgresStore(conn).sync(notes, embedder)
        finally:
            conn.close()
        print(f"Embedded {embedded} changed notes with {embedder.model_name}")


if __name__ == "__main__":
    main()
