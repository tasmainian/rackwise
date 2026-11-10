"""Scores the coach against evals/positions.json, so prompts can be tuned measurably.

    python evals/score.py --check    # no API calls: check the grader on reference answers
    python evals/score.py            # calls Claude once per position, which costs money

Run it inside the coach's virtual environment. Positions hold illustrative word lists
written by hand, so no network lookup is needed and results can be compared across runs.
Strategy notes are retrieved exactly as the app does, from an in-memory copy of notes/ with
the same local embedding model, so evals don't need Postgres. Claude gets the same trimmed
input as the app.

The grader is deliberately simple text matching. It reliably catches invented words and
numbers, wrong recommendations and forbidden topics. It can't judge whether the advice is
any good, and a correct answer in unexpected wording can fail, so read failures by hand.
"""

import argparse
import json
import logging
import sys
from datetime import UTC, datetime
from pathlib import Path

from rackwise_coach.analysis import compare, game_stats, tiles_needed
from rackwise_coach.board import ALL_TILES_BONUS, RACK_SIZE
from rackwise_coach.coaching import (
    PROMPT_VERSION,
    GameCoaching,
    build_input,
    check_coaching,
    coach_game,
)
from rackwise_coach.notes import Embedder, MemoryStore, load_notes, notes_for_turns

HERE = Path(__file__).resolve().parent


def load_positions() -> list[dict]:
    return json.loads((HERE / "positions.json").read_text())


def game_for(position: dict) -> dict:
    """Build the game the coach receives. A case is one turn, or a list of `turns`."""
    specs = position.get("turns", [position])
    turns = [_turn(spec) | {"turn": number} for number, spec in enumerate(specs, start=1)]
    return {"stats": game_stats(turns), "turns": turns}


def _turn(spec: dict) -> dict:
    rack = spec["rack"]
    letters = spec.get("board_letters", "")
    more = spec.get("more_available", False)
    placed = {w[0]: _placed(w, rack, letters) for w in spec["words"]}
    played = _played(spec.get("played_word"), placed, rack, letters)
    return compare(rack, list(placed.values()), played, more)


def _played(word: str | None, placed: dict, rack: str, letters: str) -> dict | None:
    if word is None:
        return None
    if word in placed:
        return placed[word] | {"status": "listed", "invalid_words": []}
    return _placed([word, None], rack, letters) | {"status": "not_valid", "invalid_words": [word]}


def _placed(word: list, rack: str, letters: str) -> dict:
    """Stand in for a placement. These positions only test the coach's use of the data."""
    tiles = tiles_needed(word[0], rack, letters)
    return {
        "word": word[0],
        "site_points": word[1],
        "blank_positions": word[2] if len(word) > 2 else [],
        "direction": "across",
        "position": "8H",
        "rack_tiles_used": "".join(sorted(tiles)),
        "cross_words": [],
        # Scored as board.py would, with the bonus for using every tile.
        "play_score": (word[1] or 0) + (ALL_TILES_BONUS if len(tiles) == RACK_SIZE else 0),
        "premiums_used": [],
    }


def grade(position: dict, coaching: GameCoaching, game: dict) -> list[str]:
    """Return the checks this answer failed. An empty list is a pass.

    `game` is what Claude was sent, so the check sees exactly the data it had.
    """
    notes = [note.note for note in coaching.turns]
    lowered = " ".join([coaching.summary, *coaching.themes, *notes]).lower()
    failures = [f"unsupported: {p}" for p in check_coaching(coaching, game)]
    # Whole-game cases check the habit named, not one turn's recommendation.
    if "recommend" in position:
        recommended = coaching.turns[0].recommended_word if coaching.turns else None
        # The prompt asks for no recommendation when the player's own play was best.
        played = game["turns"][0]["played"]
        if recommended is None and played and played["status"] == "listed":
            recommended = played["word"]
        if recommended not in position["recommend"]:
            failures.append(f"recommended {recommended}")
    for any_of in position["mentions"]:
        if not any(term.lower() in lowered for term in any_of):
            failures.append(f"mentioned none of {any_of}")
    for term in position["forbidden"]:
        if term.lower() in lowered:
            failures.append(f"mentioned forbidden {term!r}")
    # Retrieval is graded too: at least one of these notes must have been sent for turn 1.
    if "retrieves" in position:
        sent = game["turns"][0].get("note_ids", [])
        if not set(position["retrieves"]) & set(sent):
            failures.append(f"retrieved {sent}, not any of {position['retrieves']}")
    return failures


def main() -> int:
    # Shows each call's token counts, logged by the coach.
    logging.basicConfig(level=logging.INFO, format="%(message)s")
    logging.getLogger("httpx").setLevel(logging.WARNING)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="grade reference answers only")
    args = parser.parse_args()

    embedder = Embedder()
    store = MemoryStore(load_notes(), embedder)
    rows = []
    for position in load_positions():
        game = game_for(position)
        sent = build_input(game, notes_for_turns(game["turns"], store, embedder))
        note_ids = {turn["turn"]: turn.get("note_ids", []) for turn in sent["turns"]}
        try:
            if args.check:
                answer = GameCoaching.model_validate(position["reference"])
            else:
                answer = coach_game(sent)
            failures = grade(position, answer, sent)
            rows.append(
                {
                    "id": position["id"],
                    "failures": failures,
                    "note_ids": note_ids,
                    "answer": answer.model_dump(),
                }
            )
        except Exception as err:  # One failed call shouldn't lose the whole paid run.
            rows.append(
                {
                    "id": position["id"],
                    "failures": [f"error: {err}"],
                    "note_ids": note_ids,
                    "answer": None,
                }
            )
        status = "pass" if not rows[-1]["failures"] else "FAIL " + "; ".join(rows[-1]["failures"])
        print(f"{position['id']}: {status}")

    passed = sum(not row["failures"] for row in rows)
    print(f"\n{passed}/{len(rows)} passed (prompt {PROMPT_VERSION})")
    if not args.check:
        _save(rows, passed)
    return 0 if passed == len(rows) else 1


def _save(rows: list[dict], passed: int) -> None:
    results = HERE / "results"
    results.mkdir(exist_ok=True)
    stamp = datetime.now(UTC).strftime("%Y%m%dT%H%M%SZ")
    path = results / f"{PROMPT_VERSION}-{stamp}.json"
    path.write_text(
        json.dumps(
            {"prompt": PROMPT_VERSION, "passed": passed, "total": len(rows), "rows": rows}, indent=2
        )
    )
    print(f"Saved {path}")


if __name__ == "__main__":
    sys.exit(main())
