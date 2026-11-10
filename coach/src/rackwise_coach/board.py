"""Finds where words from word.tips fit on a real board, and checks their cross-words.

word.tips only answers "what can these letters make", so each row and column gets its
own lookup, and this code handles geometry and multipliers. It judges no words.

Requests are capped because the site's robots.txt disallows this path. A review stays
user-triggered and small, and reports what the cap left unchecked.
See docs/decisions/0008-practice-coach.md.
"""

import re
from collections import Counter

from rackwise_coach import word_lookup
from rackwise_coach.word_lookup import WordLookupError

SIZE = 15
CENTER = 7
TOP_OPTIONS = 5
# A game reviews every turn, so each turn gets a small share of requests.
MAX_LOOKUPS = 12
MAX_LINE_LOOKUPS = 8
_ROW = re.compile(r"[A-Z.]{15}")
_RACK = re.compile(r"[A-Z?]{1,7}")

# The multipliers on the player's game board, copied from their screenshot so that a review
# scores the position they actually have. T is 3W, D is 2W, t is 3L and d is 2L.
PREMIUMS = [
    "t..T...d...T..t",
    ".D....t.t....D.",
    "....d.....d....",
    "T..d...D...d..T",
    "..d..t...t..d..",
    "....t..d..t....",
    ".t...........t.",
    "d..D.d...d.D..d",
    ".t...........t.",
    "....t..d..t....",
    "..d..t...t..d..",
    "T..d...D...d..T",
    "....d.....d....",
    ".D....t.t....D.",
    "t..T...d...T..t",
]
LETTER_MULTIPLIERS = {"d": 2, "t": 3}
WORD_MULTIPLIERS = {"D": 2, "T": 3}
PREMIUM_NAMES = {"d": "2L", "t": "3L", "D": "2W", "T": "3W"}

# Solved from word.tips' word points: 614 sampled words gave one exact solution, and six
# letters matched the player's screenshot. tests/test_board.py checks them against recorded
# responses, and they must be updated if the site's points change.
TILE_VALUES = dict(
    A=1, B=4, C=3, D=2, E=1, F=4, G=4, H=3, I=1, J=10, K=6, L=2, M=3,
    N=1, O=1, P=3, Q=10, R=1, S=1, T=1, U=2, V=6, W=5, X=8, Y=4, Z=10,
)  # fmt: skip
# From third-party guides, not an official source. Added after multipliers.
ALL_TILES_BONUS = 40
RACK_SIZE = 7


def parse_board(rows: list[str]) -> list[str]:
    rows = [row.upper() for row in rows]
    if len(rows) != SIZE or not all(_ROW.fullmatch(row) for row in rows):
        raise WordLookupError("board must be 15 rows of 15 characters, using A-Z or .")
    return rows


class Lookups:
    """Makes each lookup at most once per review, within a fixed budget."""

    def __init__(self, limit: int = MAX_LOOKUPS):
        self.limit = limit
        self.used = 0
        self._words: dict[tuple[str, str], dict] = {}
        self._points: dict[str, int | None] = {}

    def spent(self) -> bool:
        return self.used >= self.limit

    def words(self, rack: str, letters: str) -> dict:
        key = (rack, "".join(sorted(letters)))
        if key not in self._words:
            self.used += 1
            self._words[key] = word_lookup.lookup_words(rack, letters)
        return self._words[key]

    def has_words(self, rack: str, letters: str) -> bool:
        return (rack, "".join(sorted(letters))) in self._words

    def points(self, word: str) -> int | None:
        """Return a cross-word's points, or None if the site doesn't list it."""
        if word not in self._points:
            self.used += 1
            self._points[word] = word_lookup.word_points(word)
        return self._points[word]


def find_candidates(rows: list[str], rack: str, lookups: Lookups) -> dict:
    """List every placement of the looked-up words, highest score first."""
    across = rows
    down = ["".join(column) for column in zip(*rows, strict=True)]
    empty = not any(set(row) - {"."} for row in rows)
    rack_only = lookups.words(rack, "")

    lines = [("across", i, across) for i in range(SIZE)] + [("down", i, down) for i in range(SIZE)]
    # Lines with more letters offer more ways to play through, so they get the budget first.
    lines.sort(key=lambda line: -len(_letters(line[2][line[1]])))

    placements = []
    more_available = rack_only["more_available"]
    skipped = 0
    for direction, index, grid in lines:
        if empty and (direction, index) != ("across", CENTER):
            continue
        letters = _letters(grid[index])
        entries = {w["word"]: w for w in rack_only["words"]}
        if letters:
            if not lookups.has_words(rack, letters) and lookups.used >= MAX_LINE_LOOKUPS:
                skipped += 1
            else:
                found = lookups.words(rack, letters)
                more_available = more_available or found["more_available"]
                entries |= {w["word"]: w for w in found["words"]}
        for entry in entries.values():
            placements += _fit(entry, direction, index, grid, rack, empty)

    placements.sort(key=lambda p: (-p["play_score"], p["word"], p["position"]))
    return {"placements": placements, "more_available": more_available, "lines_skipped": skipped}


def choose_options(placements: list[dict], lookups: Lookups) -> dict:
    """Keep the best distinct words whose cross-words are all valid."""
    options: dict[str, dict] = {}
    ran_out = False
    for placement in placements:
        if placement["word"] in options or len(options) >= TOP_OPTIONS:
            continue
        if lookups.limit - lookups.used < len(placement["cross_words"]):
            ran_out = True
            continue
        checked = _with_cross_points(placement, lookups)
        if checked is not None:
            options[placement["word"]] = checked
    ranked = sorted(options.values(), key=lambda o: (-o["play_score"], o["word"]))
    return {"options": ranked, "ran_out": ran_out}


def read_play(rows: list[str], rack: str, tiles: list[dict]) -> dict:
    """Turn the tiles a player placed into a scored play, checking only the geometry.

    Raises WordLookupError with a reason a player can act on. Word validity is checked
    when the game is reviewed, so recording a turn makes no requests.
    """
    if not _RACK.fullmatch(rack):
        raise WordLookupError("enter your rack: 1 to 7 of A-Z, with ? for a blank")
    if not tiles:
        raise WordLookupError("place at least one tile")
    placed: dict[tuple[int, int], str] = {}
    for tile in tiles:
        square = (tile["row"], tile["col"])
        letter = tile["letter"].upper()
        if not (0 <= square[0] < SIZE and 0 <= square[1] < SIZE):
            raise WordLookupError("a tile is off the board")
        if not re.fullmatch("[A-Z]", letter):
            raise WordLookupError("each tile must be a letter from A to Z")
        if rows[square[0]][square[1]] != "." or square in placed:
            raise WordLookupError("a tile is on a square that's already taken")
        placed[square] = letter

    in_rows = {row for row, _ in placed}
    in_cols = {col for _, col in placed}
    if len(in_rows) > 1 and len(in_cols) > 1:
        raise WordLookupError("all your tiles must be in one row or one column")

    down = ["".join(column) for column in zip(*rows, strict=True)]
    empty = not any(set(row) - {"."} for row in rows)
    across = _read_line("across", rows, in_rows.copy().pop(), placed) if len(in_rows) == 1 else None
    along = _read_line("down", down, in_cols.copy().pop(), placed) if len(in_cols) == 1 else None
    # One tile can lie in both directions, so its main word is whichever one forms a word.
    line = across if across and len(across[0]) > 1 else along
    if line is None or len(line[0]) < 2:
        raise WordLookupError("your play must form a word of at least two letters")

    word, direction, index, start, new, grid = line
    tiles_used = _rack_tiles(word, start, new, rack)
    if tiles_used is None:
        raise WordLookupError("your rack doesn't have the tiles for this play")
    crosses = []
    for at, tile in zip(new, tiles_used, strict=True):
        cross = _cross_word(grid, index, at, word[at - start])
        if cross:
            crosses.append((at, tile, cross))
    if empty:
        if index != CENTER or not start <= CENTER < start + len(word):
            raise WordLookupError("the first play must cover the centre square")
    elif len(new) == len(word) and not crosses:
        raise WordLookupError("your play must connect to the tiles on the board")
    entry = {"site_points": None}
    return _placement(entry, word, direction, index, start, new, tiles_used, crosses)


def check_play(placement: dict, lookups: Lookups) -> dict:
    """Ask the site about every word a recorded play formed."""
    points = lookups.points(placement["word"])
    crosses = [{"word": w, "site_points": lookups.points(w)} for w in placement["cross_words"]]
    invalid = [c["word"] for c in crosses if c["site_points"] is None]
    if points is None:
        invalid.insert(0, placement["word"])
    return placement | {
        "site_points": points,
        "cross_words": crosses,
        "invalid_words": invalid,
        "status": "not_valid" if invalid else "listed",
    }


def _read_line(direction, grid, index, placed):
    new = sorted(col if direction == "across" else row for row, col in placed)
    letters = {
        (col if direction == "across" else row): letter for (row, col), letter in placed.items()
    }
    line = list(grid[index])
    for at, letter in letters.items():
        line[at] = letter
    start, end = new[0], new[-1]
    if "." in line[start : end + 1]:
        raise WordLookupError("your tiles must form one word, with no gaps")
    while start > 0 and line[start - 1] != ".":
        start -= 1
    while end < SIZE - 1 and line[end + 1] != ".":
        end += 1
    return "".join(line[start : end + 1]), direction, index, start, new, grid


def _letters(line: str) -> str:
    return line.replace(".", "")


def _with_cross_points(placement: dict, lookups: Lookups) -> dict | None:
    crosses = []
    for word in placement["cross_words"]:
        points = lookups.points(word)
        if points is None:
            return None
        crosses.append({"word": word, "site_points": points})
    total = placement["site_points"] + sum(c["site_points"] for c in crosses)
    return placement | {"cross_words": crosses, "total_site_points": total}


def _fit(entry: dict, direction: str, index: int, grid: list[str], rack: str, empty: bool):
    word = entry["word"]
    line = grid[index]
    for start in range(SIZE - len(word) + 1):
        end = start + len(word)
        if (start > 0 and line[start - 1] != ".") or (end < SIZE and line[end] != "."):
            continue
        new = [start + k for k, letter in enumerate(word) if line[start + k] == "."]
        if not new or any(line[start + k] not in (".", letter) for k, letter in enumerate(word)):
            continue
        tiles = _rack_tiles(word, start, new, rack)
        if tiles is None or tiles.count("?") != len(entry["blank_positions"]):
            continue
        crosses = []
        for at, tile in zip(new, tiles, strict=True):
            cross = _cross_word(grid, index, at, word[at - start])
            if cross:
                crosses.append((at, tile, cross))
        through = len(new) < len(word)
        if empty:
            if not start <= CENTER < end:
                continue
        elif not through and not crosses:
            continue
        yield _placement(entry, word, direction, index, start, new, tiles, crosses)


def _rack_tiles(word: str, start: int, new: list[int], rack: str) -> list[str] | None:
    left = Counter(rack)
    tiles = []
    for at in new:
        letter = word[at - start]
        tile = letter if left[letter] > 0 else "?"
        if left[tile] == 0:
            return None
        left[tile] -= 1
        tiles.append(tile)
    return tiles


def _cross_word(grid: list[str], index: int, at: int, letter: str) -> str | None:
    """Return the word a new tile forms at right angles to the play, if any."""
    before = ""
    i = index - 1
    while i >= 0 and grid[i][at] != ".":
        before = grid[i][at] + before
        i -= 1
    after = ""
    i = index + 1
    while i < SIZE and grid[i][at] != ".":
        after += grid[i][at]
        i += 1
    return before + letter + after if before or after else None


def score_play(word, direction, index, start, new, tiles) -> tuple[int, list[str]]:
    """Return a play's main-word score and the premiums it used.

    Multipliers count only for tiles placed this turn. That's the genre's usual rule, and it
    hasn't been confirmed for the player's game. A blank scores nothing.
    """
    placed = {at: tile for at, tile in zip(new, tiles, strict=True)}
    total = 0
    word_multiplier = 1
    premiums = []
    for offset, letter in enumerate(word):
        at = start + offset
        value = TILE_VALUES[letter]
        if at in placed:
            if placed[at] == "?":
                value = 0
            premium = _premium(direction, index, at)
            value *= LETTER_MULTIPLIERS.get(premium, 1)
            word_multiplier *= WORD_MULTIPLIERS.get(premium, 1)
            if premium in PREMIUM_NAMES:
                premiums.append(PREMIUM_NAMES[premium])
        total += value
    return total * word_multiplier, premiums


def _premium(direction: str, index: int, at: int) -> str:
    return PREMIUMS[index][at] if direction == "across" else PREMIUMS[at][index]


def _cross_score(cross: str, letter: str, tile: str, premium: str) -> int:
    existing = sum(TILE_VALUES[c] for c in cross) - TILE_VALUES[letter]
    value = 0 if tile == "?" else TILE_VALUES[letter] * LETTER_MULTIPLIERS.get(premium, 1)
    return (existing + value) * WORD_MULTIPLIERS.get(premium, 1)


def _placement(entry, word, direction, index, start, new, tiles, crosses) -> dict:
    # Positions use the usual notation: 8H is row 8, column H, across. H8 is the same
    # square going down.
    column = "ABCDEFGHIJKLMNO"
    if direction == "across":
        position = f"{index + 1}{column[start]}"
    else:
        position = f"{column[index]}{start + 1}"
    blanks = [at - start for at, tile in zip(new, tiles, strict=True) if tile == "?"]

    score, premiums = score_play(word, direction, index, start, new, tiles)
    for at, tile, cross in crosses:
        score += _cross_score(cross, word[at - start], tile, _premium(direction, index, at))
    if len(tiles) == RACK_SIZE:
        score += ALL_TILES_BONUS
    return {
        "word": word,
        "site_points": entry["site_points"],
        "play_score": score,
        "premiums_used": premiums,
        "blank_positions": blanks,
        "direction": direction,
        "position": position,
        "rack_tiles_used": "".join(sorted(tiles)),
        "cross_words": [cross for _, _, cross in crosses],
    }
