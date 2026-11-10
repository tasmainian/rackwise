"""Turns placed words into the facts the coach explains. Nothing here calls an LLM.

Validity and base points come from word.tips, and scores come from board.py.
See docs/decisions/0008-practice-coach.md.
"""

from collections import Counter

from rackwise_coach.word_lookup import WordLookupError

VOWELS = set("AEIOU")
# A turn at least this many points behind the best play found gets strategy notes.
# Smaller gaps can be a fair trade for a better leave, so they aren't called mistakes.
MISTAKE_POINTS = 6
# Problems with a play's leave or with the tiles it spent. Each is a lesson even when the
# score was close, but only if another play without that problem cost fewer than
# MISTAKE_POINTS points.
_PLAY_PROBLEMS = {
    "q_without_u": lambda play: play["leave_facts"]["q_without_u"],
    "blank_spent": lambda play: "?" in play["rack_tiles_used"] and not play["uses_whole_rack"],
    "duplicates": lambda play: bool(play["leave_facts"]["duplicate_letters"]),
    "vowel_heavy": lambda play: (
        play["leave_facts"]["vowels"] >= max(3, play["leave_facts"]["consonants"] + 2)
    ),
    # A blank can be any letter, so it balances two consonants here.
    "consonant_heavy": lambda play: (
        play["leave_facts"]["consonants"] >= 3
        and play["leave_facts"]["consonants"]
        >= play["leave_facts"]["vowels"] + 2 * play["leave_facts"]["blanks"] + 2
    ),
    "s_spent": lambda play: "S" in play["rack_tiles_used"] and not play["uses_whole_rack"],
}
# Roughly how many points each problem tends to cost, biggest first. Retrieval takes one
# note per tag in this order, so the costliest problems get notes first.
_TAG_PRIORITY = [
    "invalid_play",
    "missed_bingo",
    "swap",
    "premium_missed",
    "q_without_u",
    "blank_kept",
    "blank_spent",
    "duplicates",
    "vowel_heavy",
    "consonant_heavy",
    "s_spent",
    "low_score",
]
_QUERY_PHRASES = {
    "swap": "Swapped or passed instead of playing.",
    "invalid_play": "Played a word that was not valid.",
    "low_score": "Chose a lower-scoring play.",
    "duplicates": "Kept duplicate letters.",
    "vowel_heavy": "Kept too many vowels.",
    "consonant_heavy": "Kept consonants with no vowels.",
    "q_without_u": "Kept a Q without a U.",
    "blank_kept": "Held the blank instead of spending it for a big score.",
    "blank_spent": "Spent the blank on a small play.",
    "s_spent": "Spent an S on a small play.",
    "missed_bingo": "Missed a play using all seven tiles.",
    "premium_missed": "Missed a multiplier square.",
}


def compare(
    rack: str,
    options: list[dict],
    played: dict | None = None,
    more_available: bool = False,
    lines_skipped: int = 0,
) -> dict:
    """Describe each option's leave, and how far the played word is behind the best."""
    described = [_describe(option, rack) for option in options]
    return {
        "rack": rack,
        "more_available": more_available,
        "lines_skipped": lines_skipped,
        "options": described,
        "played": _played(played, described, rack) if played else None,
    }


def game_stats(turns: list[dict]) -> dict:
    """Add up a game's turns. These are the only totals the coach may quote."""
    played = [t["played"] for t in turns if t["played"]]
    valid = [p for p in played if p["status"] == "listed"]
    behind = [p["points_behind_best"] for p in valid]
    numbered = [(t["turn"], t["played"]) for t in turns if t["played"]]
    misses = [(p["points_behind_best"], turn) for turn, p in numbered if p["status"] == "listed"]
    biggest = max(misses, default=(0, None))
    return {
        "turns": len(turns),
        "swaps_or_passes": len(turns) - len(played),
        "invalid_plays": len(played) - len(valid),
        "total_score": sum(p["play_score"] for p in valid),
        "total_points_behind": sum(behind),
        "average_points_behind": round(sum(behind) / len(behind), 1) if behind else 0,
        "biggest_miss_points": biggest[0],
        "biggest_miss_turn": biggest[1],
        "all_tile_plays": sum(p["uses_whole_rack"] for p in valid),
    }


def mistake_tags(turn: dict) -> list[str]:
    """Label what went wrong on a turn, most costly first, for choosing strategy notes.

    [] means there's no lesson. Plain code decides this from the turn's facts, so retrieval
    never depends on the LLM.
    """
    played = turn["played"]
    options = turn["options"]
    if played is not None and played["status"] != "listed":
        return ["invalid_play"]
    best = max(options, key=lambda option: option["play_score"], default=None)
    if played is None:
        missed = best is not None and best["uses_whole_rack"]
        return ["missed_bingo", "swap"] if missed else ["swap"]
    if best is None:
        return []
    found = set()
    if played["points_behind_best"] >= MISTAKE_POINTS:
        checks = {
            "missed_bingo": best["uses_whole_rack"] and not played["uses_whole_rack"],
            "premium_missed": bool(best["premiums_used"]) and not played["premiums_used"],
            # Holding the blank only counts against the player when the best play spent it.
            "blank_kept": played["leave_facts"]["blanks"] > 0 and "?" in best["rack_tiles_used"],
            "low_score": True,
        }
        found |= {tag for tag, applies in checks.items() if applies}
    for tag, has_problem in _PLAY_PROBLEMS.items():
        if has_problem(played) and any(
            not has_problem(option) and option["play_score"] > played["play_score"] - MISTAKE_POINTS
            for option in options
        ):
            found.add(tag)
    return [tag for tag in _TAG_PRIORITY if tag in found]


def mistake_query(turn: dict, tag: str) -> str:
    """Describe one problem in words, to rank the notes that share its tag by similarity.

    The turn's details steer the ranking, for example a doubled vowel towards the note on
    doubled vowels rather than the general one on duplicates.
    """
    played = turn["played"]
    parts = [_QUERY_PHRASES[tag]]
    facts = played["leave_facts"] if played else None
    if tag == "duplicates" and facts:
        for letter in facts["duplicate_letters"]:
            kind = "vowel" if letter in VOWELS else "consonant"
            parts.append(f"Kept two {letter}'s, a doubled {kind}.")
    elif tag in ("vowel_heavy", "consonant_heavy") and facts:
        parts.append(f"Kept {facts['vowels']} vowels and {facts['consonants']} consonants.")
    elif tag == "swap":
        best = max((option["play_score"] for option in turn["options"]), default=0)
        parts.append("A decent play was available." if best else "No play was found.")
    elif tag == "s_spent" and facts and "S" in facts["duplicate_letters"]:
        parts.append("Still held another S.")
    return " ".join(parts)


def tiles_needed(word: str, rack: str, board_letters: str) -> list[str]:
    """Work out the rack tiles a word needs, using board letters wherever possible."""
    board_left = Counter(board_letters)
    rack_left = Counter(rack)
    used = []
    for letter in word:
        if board_left[letter] > 0:
            board_left[letter] -= 1
            continue
        tile = letter if rack_left[letter] > 0 else "?"
        if rack_left[tile] == 0:
            raise WordLookupError(f"{word} can't be made from these letters")
        rack_left[tile] -= 1
        used.append(tile)
    if not used:
        raise WordLookupError(f"{word} must use at least one tile from the rack")
    return used


def _played(played: dict, options: list[dict], rack: str) -> dict:
    described = _describe(played, rack)
    if played["status"] != "listed":
        return described | {"points_behind_best": None}
    # The search can miss plays, so the player's own play can be the best one known.
    best = max([o["play_score"] for o in options] + [described["play_score"]])
    return described | {"points_behind_best": best - described["play_score"]}


def _describe(option: dict, rack: str) -> dict:
    used = option["rack_tiles_used"]
    leave = "".join(sorted((Counter(rack) - Counter(used)).elements()))
    crosses = [c["word"] if isinstance(c, dict) else c for c in option["cross_words"]]
    return option | {
        "words_formed": [option["word"], *crosses],
        "uses_whole_rack": len(used) == len(rack) == 7,
        "leave": leave,
        "leave_facts": _leave_facts(leave),
    }


def _leave_facts(leave: str) -> dict:
    counts = Counter(leave)
    return {
        "vowels": sum(counts[v] for v in VOWELS),
        "consonants": sum(n for t, n in counts.items() if t not in VOWELS and t != "?"),
        "blanks": counts["?"],
        "duplicate_letters": sorted(t for t, n in counts.items() if n > 1 and t != "?"),
        "q_without_u": counts["Q"] > 0 and counts["U"] == 0,
    }
