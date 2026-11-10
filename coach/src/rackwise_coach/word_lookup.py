"""Word lookups on word.tips, for checking which words NWL23 accepts.

This is a development aid and has no say in the game: the engine still decides what
is legal and what a play scores. See docs/decisions/0007-word-additions.md.

The site's robots.txt disallows this path for crawlers, so the code stays deliberately
small-scale: at most one request a second, nothing stored, and later result pages are
never fetched.
"""

import re
import threading
import time
from collections import Counter
from typing import Any

import httpx

URL = "https://word.tips/unscramble/results/__data.json"
DICTIONARY = "nwl23"
RACK_SIZE = 7
MAX_BOARD_LETTERS = 15
MIN_SECONDS_BETWEEN_REQUESTS = 1.0

_RACK = re.compile(r"[A-Z?]+")
_BOARD = re.compile(r"[A-Z]*")
_WORD = re.compile(r"[a-z]+")

_lock = threading.Lock()
_last_request = 0.0


class WordLookupError(Exception):
    """The input was invalid."""


class WordSiteError(WordLookupError):
    """The site failed, or its answer couldn't be used."""


def lookup_words(rack: str, board_letters: str = "", client: httpx.Client | None = None) -> dict:
    """Return the NWL23 words that can be made from the rack and board letters.

    A word made only from board letters is left out, since a play must use a rack tile.
    """
    rack = rack.upper()
    board_letters = board_letters.upper()
    _check_letters(rack, board_letters)

    response = _fetch(rack + board_letters, client)
    pages = _word_pages(response)

    words = []
    site_total = 0
    for page in pages:
        site_total += page["num_words"]
        for entry in page["word_list"]:
            if _uses_rack_tile(entry["word"], entry["wildcards"], board_letters):
                words.append(
                    {
                        "word": entry["word"].upper(),
                        "site_points": entry["points"],
                        "blank_positions": entry["wildcards"],
                    }
                )
    words.sort(key=lambda w: (-w["site_points"], w["word"]))
    return {
        "letters": rack + board_letters,
        "dictionary": DICTIONARY,
        "words": words,
        "site_total": site_total,
        "more_available": any(page["num_pages"] > 1 for page in pages),
    }


def word_points(word: str, client: httpx.Client | None = None) -> int | None:
    """Return a word's points, or None if the site doesn't list it."""
    if not _BOARD.fullmatch(word) or not 2 <= len(word) <= MAX_BOARD_LETTERS:
        raise WordLookupError("word must be 2 to 15 of A-Z")
    for page in _word_pages(_fetch(word, client)):
        for entry in page["word_list"]:
            if entry["word"].upper() == word and not entry["wildcards"]:
                return entry["points"]
    return None


def _check_letters(rack: str, board_letters: str) -> None:
    if not _RACK.fullmatch(rack) or len(rack) > RACK_SIZE:
        raise WordLookupError(f"rack must be 1 to {RACK_SIZE} of A-Z or ?")
    if not _BOARD.fullmatch(board_letters) or len(board_letters) > MAX_BOARD_LETTERS:
        raise WordLookupError(f"board_letters must be at most {MAX_BOARD_LETTERS} of A-Z")


def _fetch(letters: str, client: httpx.Client | None) -> Any:
    params = {
        "letters": letters,
        "dictionary": DICTIONARY,
        "word_sorting": "points",
        "x-sveltekit-trailing-slash": "1",
        "x-sveltekit-invalidated": "001",
    }
    owned = client is None
    client = client or httpx.Client(timeout=10, headers={"User-Agent": "rackwise-dev-lookup"})
    try:
        _wait_for_turn()
        response = client.get(URL, params=params)
        response.raise_for_status()
        return response.json()
    except (httpx.HTTPError, ValueError) as err:
        raise WordSiteError(f"word.tips request failed: {err}") from err
    finally:
        if owned:
            client.close()


def _wait_for_turn() -> None:
    global _last_request
    with _lock:
        wait = _last_request + MIN_SECONDS_BETWEEN_REQUESTS - time.monotonic()
        if wait > 0:
            time.sleep(wait)
        _last_request = time.monotonic()


def _word_pages(response: Any) -> list[dict]:
    """Decode the response and check its shape, since the endpoint is undocumented."""
    try:
        nodes = [n for n in response["nodes"] if n and n.get("type") == "data"]
        pages = _hydrate(nodes[-1]["data"], 0)["wordPages"]
        for page in pages:
            for key in ("num_words", "num_pages"):
                if not isinstance(page[key], int):
                    raise TypeError(key)
            for entry in page["word_list"]:
                # Only plain words get through, as this text may reach an LLM.
                if not _WORD.fullmatch(entry["word"]) or not isinstance(entry["points"], int):
                    raise ValueError(entry["word"])
                if not all(isinstance(i, int) for i in entry["wildcards"]):
                    raise TypeError("wildcards")
        return pages
    except (KeyError, IndexError, TypeError, ValueError, AttributeError, RecursionError) as err:
        raise WordSiteError(f"unexpected word.tips response: {err!r}") from err


def _hydrate(data: list, index: int) -> Any:
    """Rebuild a value from SvelteKit's flat format, where containers hold indexes.

    Negative indexes are the format's markers for values such as undefined.
    """
    if index < 0:
        return None
    value = data[index]
    if isinstance(value, dict):
        return {key: _hydrate(data, i) for key, i in value.items()}
    if isinstance(value, list):
        return [_hydrate(data, i) for i in value]
    return value


def _uses_rack_tile(word: str, blank_positions: list[int], board_letters: str) -> bool:
    if blank_positions:
        return True  # Blanks only come from the rack.
    return bool(Counter(word.upper()) - Counter(board_letters))
