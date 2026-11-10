import logging
import os

import anthropic
from fastapi import FastAPI
from fastapi.responses import JSONResponse
from pydantic import BaseModel

from rackwise_coach import analysis, board, board_reader, coaching, notes, word_lookup

# The coach logs each Claude call's token counts at INFO, which uvicorn doesn't show by default.
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)
app = FastAPI(title="Rackwise Coach")
# Loaded on the first summary, then kept for the life of the process.
_embedder = notes.Embedder()


class Tile(BaseModel):
    row: int
    col: int
    letter: str


class PlayRequest(BaseModel):
    rack: str
    board: list[str]
    tiles: list[Tile]


class ReviewRequest(BaseModel):
    rack: str
    board: list[str]
    tiles: list[Tile] = []


class SummaryRequest(BaseModel):
    turns: list[dict]


class ReadBoardRequest(BaseModel):
    image_base64: str


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok", "service": "coach"}


@app.get("/v1/practice/premiums")
def premiums() -> dict[str, list[str]]:
    return {"premiums": board.PREMIUMS}


@app.post("/v1/practice/read-board")
def read_board(request: ReadBoardRequest):
    """Read the tiles and rack from a picture, for the player to check on the grid."""
    if not _has_credentials():
        message = "Reading a picture needs an ANTHROPIC_API_KEY. You can still type the tiles in."
        return JSONResponse({"error": message}, status_code=503)
    try:
        return board_reader.read_board(request.image_base64)
    except board_reader.BoardImageError as err:
        return JSONResponse({"error": str(err)}, status_code=422)
    except (board_reader.BoardReadError, anthropic.APIError) as err:
        logger.warning("board reading failed: %s", err)
        message = "That picture couldn't be read. Try again, or type the tiles in."
        return JSONResponse({"error": message}, status_code=502)


@app.post("/v1/practice/check-play")
def check_play(request: PlayRequest):
    """Read and score a play while recording a turn, with no requests to the word site."""
    try:
        rows = board.parse_board(request.board)
        rack = request.rack.upper()
        play = board.read_play(rows, rack, [tile.model_dump() for tile in request.tiles])
    except word_lookup.WordLookupError as err:
        return JSONResponse({"error": str(err)}, status_code=422)
    words = [play["word"], *play["cross_words"]]
    return {"words_formed": words, "position": play["position"], "play_score": play["play_score"]}


@app.post("/v1/practice/review")
def review(request: ReviewRequest):
    """Compare one recorded turn with the best plays found. This doesn't call Claude."""
    try:
        return _compare(request)
    except word_lookup.WordSiteError as err:
        return JSONResponse({"error": str(err)}, status_code=502)
    except word_lookup.WordLookupError as err:
        return JSONResponse({"error": str(err)}, status_code=422)


@app.post("/v1/practice/summary")
def summary(request: SummaryRequest):
    """Coach the finished game in one Claude call. The stats are returned either way."""
    try:
        game = {"stats": analysis.game_stats(request.turns), "turns": request.turns}
    except (KeyError, TypeError) as err:
        return JSONResponse({"error": f"malformed turn: {err}"}, status_code=422)
    stats = game["stats"]
    if not _has_credentials():
        return {"stats": stats, "coaching": None, "coach_status": "no_api_key"}
    found, bodies = _strategy_notes(request.turns)
    try:
        result = coaching.coach_game(coaching.build_input(game, found))
    except coaching.CoachRejected as err:
        logger.warning("coaching withheld: %s", err)
        return {"stats": stats, "coaching": None, "coach_status": "rejected"}
    except (coaching.CoachError, anthropic.APIError) as err:
        logger.warning("coaching failed: %s", err)
        return {"stats": stats, "coaching": None, "coach_status": "error"}
    cited = {note_id for turn in result.turns for note_id in turn.cited_notes}
    shown = {note_id: body for note_id, body in bodies.items() if note_id in cited}
    return {"stats": stats, "coaching": result.model_dump(), "coach_status": "ok", "notes": shown}


def _strategy_notes(turns: list[dict]) -> tuple[dict[int, list[dict]], dict[str, dict]]:
    """Notes for each mistake turn, and the full text of each, read before the Claude call.

    Notes improve the coaching but aren't needed for it, so any failure here (no database,
    no model download) is logged and the game is coached without them.
    """
    if not os.environ.get("DATABASE_URL"):
        return {}, {}
    try:
        conn = notes.connect()
    except Exception as err:  # Deliberately broad: see the docstring.
        logger.warning("strategy notes unavailable: %s", err)
        return {}, {}
    try:
        store = notes.PostgresStore(conn)
        # Cheap when nothing changed: only edited notes are embedded again.
        store.sync(notes.load_notes(), _embedder)
        found = notes.notes_for_turns(turns, store, _embedder)
        ids = sorted({note["id"] for briefs in found.values() for note in briefs})
        return found, store.bodies(ids)
    except Exception as err:  # Deliberately broad: see the docstring.
        logger.warning("strategy notes unavailable: %s", err)
        return {}, {}
    finally:
        conn.close()


def _has_credentials() -> bool:
    return bool(os.environ.get("ANTHROPIC_API_KEY") or os.environ.get("ANTHROPIC_AUTH_TOKEN"))


def _compare(request: ReviewRequest) -> dict:
    rack = request.rack.upper()
    rows = board.parse_board(request.board)
    placement = None
    if request.tiles:
        placement = board.read_play(rows, rack, [tile.model_dump() for tile in request.tiles])

    lookups = board.Lookups()
    # The played words go first, so that the player's own play always gets checked.
    played = board.check_play(placement, lookups) if placement else None
    found = board.find_candidates(rows, rack, lookups)
    chosen = board.choose_options(found["placements"], lookups)
    more_available = found["more_available"] or found["lines_skipped"] > 0 or chosen["ran_out"]
    return analysis.compare(rack, chosen["options"], played, more_available, found["lines_skipped"])
