"""Asks Claude to summarise a finished game, then checks the answer against the data."""

import json
import logging
import os
import re
from collections.abc import Iterator
from pathlib import Path
from typing import Any

import anthropic
from pydantic import BaseModel, ValidationError

from rackwise_coach import analysis

logger = logging.getLogger(__name__)
DEFAULT_MODEL = "claude-sonnet-5-5"
PROMPT_VERSION = "game-v3"
EFFORTS = {"low", "medium", "high"}
# Turns with no lesson keep only the best play, because the coach only needs to confirm the
# choice. Mistake turns keep five, because a play with a better leave is often fourth or fifth.
OPTIONS_FOR_MISTAKES = 5
OPTIONS_OTHERWISE = 1
# The only fields the prompt uses. Board details such as blank_positions stay out, to save tokens.
_OPTION_FIELDS = (
    "word",
    "position",
    "words_formed",
    "play_score",
    "premiums_used",
    "uses_whole_rack",
    "leave",
    "leave_facts",
)
_PLAYED_FIELDS = (*_OPTION_FIELDS, "status", "invalid_words", "points_behind_best")
_DEFAULT_PROMPTS = Path(__file__).resolve().parents[3] / "prompts"
_PREMIUM_NAMES = {"2W", "3W", "2L", "3L"}


class TurnNote(BaseModel):
    turn: int
    recommended_word: str | None
    note: str
    cited_notes: list[str] = []


class GameCoaching(BaseModel):
    summary: str
    themes: list[str]
    turns: list[TurnNote]


class CoachError(Exception):
    """Claude gave no usable answer."""


class CoachRejected(CoachError):
    """Claude's answer used a word or number that isn't in the data."""


def load_prompt(version: str = PROMPT_VERSION) -> str:
    prompts = Path(os.environ.get("PROMPTS_DIR", _DEFAULT_PROMPTS))
    return (prompts / "coach" / f"{version}.md").read_text()


def build_input(game: dict, notes: dict[int, list[dict]] | None = None) -> dict:
    """Trim a game to what the prompt uses, and attach strategy notes to mistake turns.

    Each note is sent once in `strategy_notes`, and turns refer to it by ID, because the same
    note often fits several turns. check_coaching then checks against exactly this.
    """
    notes = notes or {}
    briefs = {}
    turns = []
    for turn in game["turns"]:
        found = notes.get(turn["turn"], [])
        briefs.update((note["id"], note) for note in found)
        limit = OPTIONS_FOR_MISTAKES if analysis.mistake_tags(turn) else OPTIONS_OTHERWISE
        played = turn["played"]
        trimmed = {
            "turn": turn["turn"],
            "rack": turn["rack"],
            "more_available": turn["more_available"],
            "options": [_pick(option, _OPTION_FIELDS) for option in turn["options"][:limit]],
            "played": _pick(played, _PLAYED_FIELDS) if played else None,
        }
        if found:
            trimmed["note_ids"] = [note["id"] for note in found]
        turns.append(trimmed)
    sent = {"stats": game["stats"], "turns": turns}
    if briefs:
        sent["strategy_notes"] = list(briefs.values())
    return sent


def coach_game(game: dict, client: anthropic.Anthropic | None = None) -> GameCoaching:
    """Make one Claude call for the whole game. `game` is the output of build_input."""
    effort = _effort()
    client = client or anthropic.Anthropic()
    response = client.beta.messages.create(
        model=os.environ.get("COACH_MODEL", DEFAULT_MODEL),
        max_tokens=16000,
        betas=["server-side-fallback-2026-07-01"],
        fallbacks="default",
        output_config={
            "effort": effort,
            "format": {"type": "json_schema", "schema": anthropic.transform_schema(GameCoaching)},
        },
        system=load_prompt(),
        # Compact JSON: the spaces after commas and colons were about 7% of the input tokens.
        messages=[{"role": "user", "content": json.dumps(game, separators=(",", ":"))}],
    )
    log_usage(logger, "game summary", response)
    # A refusal or a cut-off answer has nothing usable to parse.
    text = "".join(block.text for block in response.content if block.type == "text")
    try:
        coaching = GameCoaching.model_validate_json(text)
    except ValidationError as err:
        raise CoachError(f"no usable coaching (stop reason: {response.stop_reason})") from err
    problems = check_coaching(coaching, game)
    if problems:
        raise CoachRejected("; ".join(problems))
    return coaching


def log_usage(log: logging.Logger, call: str, response: Any) -> None:
    """Log a call's token counts, so real costs can be measured. Nothing else is logged."""
    usage = response.usage
    log.info(
        "%s used %s input and %s output tokens on %s",
        call,
        usage.input_tokens,
        usage.output_tokens,
        response.model,
    )


def check_coaching(coaching: GameCoaching, game: dict) -> list[str]:
    """List anything the coach said that the data doesn't back up.

    This is the enforcement behind "the LLM only explains": a recommendation, game word
    or number from outside the data withholds the whole answer.
    """
    turns = {turn["turn"]: turn for turn in game["turns"]}
    known_text = set(_PREMIUM_NAMES) | {note["id"] for note in game.get("strategy_notes", [])}
    known_numbers = {str(number) for number in turns}
    for value in _numbers(game):
        known_numbers |= set(re.findall(r"\d+", str(value)))
    for turn in game["turns"]:
        # The rack is sometimes written without its blank, which invents nothing.
        known_text |= {turn["rack"], turn["rack"].replace("?", "")}
        for described in _described(turn):
            known_text.update(described.get("words_formed", [described["word"]]))
            known_text.update(described.get("invalid_words", []))
            if described.get("leave"):
                known_text.add(described["leave"])
            known_text.add(described.get("position", ""))

    problems = []
    for note in coaching.turns:
        turn = turns.get(note.turn)
        if turn is None:
            problems.append(f"wrote a note for turn {note.turn}, which doesn't exist")
        else:
            if note.recommended_word and note.recommended_word not in _recommendable(turn):
                problems.append(f"recommended {note.recommended_word}, which isn't listed")
            for cited in sorted(set(note.cited_notes) - set(turn.get("note_ids", []))):
                problems.append(f"cited {cited}, which wasn't sent for turn {note.turn}")
    text = " ".join([coaching.summary, *coaching.themes, *(n.note for n in coaching.turns)])
    for word in sorted(set(re.findall(r"\b[A-Z0-9]{2,}\b", text))):
        if word.isdigit() or word in known_text:
            continue
        problems.append(f"mentioned {word}, which isn't in the data")
    # Digits inside labels such as 3W and H8 were checked above as whole labels.
    for number in sorted(set(re.findall(r"(?<![A-Z])\d+(?![A-Z])", text)) - known_numbers):
        problems.append(f"mentioned {number}, which isn't in the data")
    return problems


def _effort() -> str:
    effort = os.environ.get("COACH_EFFORT", "medium")
    if effort not in EFFORTS:
        raise CoachError(f"COACH_EFFORT must be one of {sorted(EFFORTS)}, not {effort!r}")
    return effort


def _pick(value: dict, fields: tuple[str, ...]) -> dict:
    return {field: value[field] for field in fields if field in value}


def _described(turn: dict) -> list[dict]:
    return [*turn["options"], *([turn["played"]] if turn["played"] else [])]


def _recommendable(turn: dict) -> set[str]:
    listed = {option["word"] for option in turn["options"]}
    played = turn["played"]
    if played and played["status"] == "listed":
        listed.add(played["word"])
    return listed


def _numbers(value: Any) -> Iterator[int | float]:
    if isinstance(value, bool):
        return
    if isinstance(value, int | float):
        yield value
    elif isinstance(value, dict):
        for item in value.values():
            yield from _numbers(item)
    elif isinstance(value, list):
        for item in value:
            yield from _numbers(item)
