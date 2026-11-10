import json
from pathlib import Path

import pytest

from rackwise_coach import board, word_lookup
from rackwise_coach.board import Lookups, choose_options, find_candidates, parse_board, read_play
from rackwise_coach.word_lookup import WordLookupError

# CAT goes across row 8, on columns G, H and I.
CAT = ["." * 15] * 7 + ["......CAT......"] + ["." * 15] * 7
EMPTY = ["." * 15] * 15
VALID_CROSSES = {"AE": 2, "CATS": 6}


@pytest.fixture
def site(monkeypatch):
    """A stand-in for word.tips, with a hand-written word list and a request log."""
    words = {"CATS": 6, "SEC": 5, "ES": 2, "SET": 3, "SE": 2}
    calls = []

    def lookup(rack, letters):
        calls.append(rack + letters)
        pool = rack + letters
        found = [w for w in words if all(w.count(c) <= pool.count(c) for c in w)]
        return {
            "dictionary": "nwl23",
            "more_available": False,
            "words": [{"word": w, "site_points": words[w], "blank_positions": []} for w in found],
        }

    monkeypatch.setattr(word_lookup, "lookup_words", lookup)
    monkeypatch.setattr(word_lookup, "word_points", VALID_CROSSES.get)
    return calls


def placements(rows, rack="SE"):
    return find_candidates(rows, rack, Lookups())["placements"]


def at(found, word):
    return {p["position"] for p in found if p["word"] == word}


def test_a_hook_extends_the_word_on_the_board(site):
    found = [p for p in placements(CAT) if p["word"] == "CATS" and p["direction"] == "across"]

    assert len(found) == 1
    assert found[0]["position"] == "8G"
    assert found[0]["rack_tiles_used"] == "S"


def test_a_word_plays_through_a_board_letter(site):
    found = next(p for p in placements(CAT) if p["word"] == "SEC")

    assert found["position"] == "G6"  # S on row 6, E on row 7, then the board's C on row 8.
    assert found["direction"] == "down"
    assert found["rack_tiles_used"] == "ES"


def test_a_parallel_play_records_its_cross_words(site):
    found = [p for p in placements(CAT) if p["word"] == "ES" and p["position"] == "9H"]

    assert found[0]["cross_words"] == ["AE", "TS"]


def test_words_that_dont_touch_the_board_are_left_out(site):
    positions = at(placements(CAT), "ES")

    assert "1A" not in positions
    assert "6H" not in positions
    assert all(position[:-1] in {"7", "9"} or position[0] in "FGHIJ" for position in positions)


def test_a_word_cant_run_into_a_tile_or_overwrite_one(site):
    positions = at(placements(CAT), "SET")

    assert "8A" not in positions
    assert "8E" not in positions  # Would end against the C.
    assert "8G" not in positions  # Would overwrite C and A.


def test_the_first_play_must_cover_the_centre(site):
    positions = at(placements(EMPTY), "ES")

    assert positions == {"8G", "8H"}


def test_invalid_cross_words_rule_out_a_placement(site):
    lookups = Lookups()
    found = find_candidates(CAT, "SE", lookups)["placements"]
    invalid_only = [p for p in found if p["word"] == "ES" and p["position"] == "9H"]

    assert choose_options(invalid_only, lookups)["options"] == []


def test_totals_add_the_sites_cross_word_points(site):
    placement = {
        "word": "S",
        "site_points": 1,
        "play_score": 9,
        "position": "8J",
        "cross_words": ["CATS", "AE"],
        "rack_tiles_used": "S",
    }
    chosen = choose_options([placement], Lookups())["options"][0]

    assert chosen["total_site_points"] == 1 + 6 + 2
    assert chosen["cross_words"][0] == {"word": "CATS", "site_points": 6}


def test_options_are_distinct_words_ranked_by_score(site):
    def place(word, points, score, crosses=()):
        return {
            "word": word,
            "site_points": points,
            "play_score": score,
            "position": "8H",
            "cross_words": list(crosses),
        }

    candidates = [place("SEC", 5, 5), place("SEC", 5, 5), place("SE", 2, 8, ["CATS"])]
    chosen = choose_options(candidates, Lookups())["options"]

    assert [o["word"] for o in chosen] == ["SE", "SEC"]
    assert [o["total_site_points"] for o in chosen] == [8, 5]


def test_a_blank_covers_a_missing_letter(monkeypatch):
    def lookup(rack, letters):
        entry = {"word": "CATS", "site_points": 5, "blank_positions": [3]}
        return {"dictionary": "nwl23", "more_available": False, "words": [entry]}

    monkeypatch.setattr(word_lookup, "lookup_words", lookup)
    found = [p for p in placements(CAT, rack="?") if p["position"] == "8G"]

    assert found[0]["rack_tiles_used"] == "?"
    assert found[0]["blank_positions"] == [3]


def test_a_full_budget_is_never_exceeded_and_reports_the_gap(site):
    rows = ["A" + "." * 14] + ["." * 15] * 14
    many = [
        "".join(chr(65 + (r + c) % 26) if (r + c) % 2 == 0 else "." for c in range(15))
        for r in range(15)
    ]
    lookups = Lookups()

    found = find_candidates(many, "SE", lookups)

    assert lookups.used <= board.MAX_LINE_LOOKUPS
    assert len(site) == lookups.used
    assert found["lines_skipped"] > 0
    assert find_candidates(rows, "SE", Lookups())["lines_skipped"] == 0


def test_same_letters_on_different_lines_share_one_lookup(site):
    rows = ["." * 15] * 7 + ["......CAT......"] + [".TAC..........."] + ["." * 15] * 6
    lookups = Lookups()

    find_candidates(rows, "SE", lookups)

    assert sum(sorted(call) == sorted("SECAT") for call in site) == 1


def test_choose_stops_checking_cross_words_when_the_budget_runs_out(site):
    lookups = Lookups(limit=0)
    placement = {"word": "ES", "site_points": 2, "position": "9H", "cross_words": ["AE"]}

    chosen = choose_options([placement], lookups)

    assert chosen["options"] == []
    assert chosen["ran_out"] is True


def tiles(*placed):
    return [{"row": row, "col": col, "letter": letter} for row, col, letter in placed]


def test_reading_a_hook_finds_the_whole_word_and_its_position():
    play = read_play(CAT, "SEDRINU", tiles((7, 9, "S")))

    assert play["word"] == "CATS"
    assert play["position"] == "8G"
    assert play["direction"] == "across"
    assert play["rack_tiles_used"] == "S"
    assert play["play_score"] == 7


def test_reading_a_stacked_play_finds_every_word_formed():
    play = read_play(CAT, "ES", tiles((8, 7, "E"), (8, 8, "S")))

    assert play["word"] == "ES"
    assert play["position"] == "9H"
    assert play["cross_words"] == ["AE", "TS"]
    assert play["play_score"] == 6


def test_reading_a_down_play_through_a_board_letter():
    play = read_play(CAT, "SE", tiles((5, 6, "S"), (6, 6, "E")))

    assert play["word"] == "SEC"
    assert play["direction"] == "down"
    assert play["position"] == "G6"
    assert play["play_score"] == 5


def test_a_single_tile_uses_the_direction_that_forms_a_word():
    play = read_play(CAT, "T", tiles((8, 7, "T")))

    assert play["word"] == "AT"
    assert play["direction"] == "down"
    assert play["position"] == "H8"
    assert play["play_score"] == 2


def test_a_letter_missing_from_the_rack_uses_the_blank():
    play = read_play(CAT, "?", tiles((7, 9, "S")))

    assert play["rack_tiles_used"] == "?"
    assert play["blank_positions"] == [3]
    assert play["play_score"] == 5


def test_reading_the_first_play_needs_the_centre_square():
    assert read_play(EMPTY, "HI", tiles((7, 7, "H"), (7, 8, "I")))["word"] == "HI"
    with pytest.raises(WordLookupError, match="centre"):
        read_play(EMPTY, "HI", tiles((3, 3, "H"), (3, 4, "I")))


@pytest.mark.parametrize(
    ("rack", "placed", "reason"),
    [
        ("SE", [], "at least one"),
        ("", [(7, 9, "S")], "rack"),
        ("S1", [(7, 9, "S")], "rack"),
        ("Z", [(7, 9, "S")], "doesn't have"),
        ("SE", [(7, 9, "S"), (8, 10, "E")], "one row or one column"),
        ("SE", [(7, 9, "S"), (7, 11, "E")], "no gaps"),
        ("SE", [(7, 6, "S")], "already taken"),
        ("SE", [(7, 9, "S"), (7, 9, "E")], "already taken"),
        ("SE", [(0, 0, "S"), (0, 1, "E")], "connect"),
        ("SE", [(7, 15, "S")], "off the board"),
        ("SE", [(7, 9, "7")], "letter"),
    ],
)
def test_reading_explains_an_illegal_play(rack, placed, reason):
    with pytest.raises(WordLookupError, match=reason):
        read_play(CAT, rack, tiles(*placed))


def test_a_lone_tile_with_no_neighbours_is_not_a_word():
    with pytest.raises(WordLookupError, match="at least two"):
        read_play(CAT, "S", tiles((0, 0, "S")))


def test_checking_a_play_reports_each_invalid_word(site):
    play = read_play(CAT, "ES", tiles((8, 7, "E"), (8, 8, "S")))
    monkey_valid = {"ES": 2, "AE": 2}
    word_lookup.word_points = monkey_valid.get

    checked = board.check_play(play, Lookups())

    assert checked["status"] == "not_valid"
    assert checked["invalid_words"] == ["TS"]
    assert checked["cross_words"][0] == {"word": "AE", "site_points": 2}


def test_checking_a_valid_play(site):
    play = read_play(CAT, "S", tiles((7, 9, "S")))

    checked = board.check_play(play, Lookups())

    assert checked["status"] == "listed"
    assert checked["invalid_words"] == []
    assert checked["site_points"] == 6


def _scored(rows, rack, word, position):
    return next(
        p for p in placements(rows, rack) if p["word"] == word and p["position"] == position
    )


def _site_listing(monkeypatch, *words):
    def lookup(rack, letters):
        entries = [{"word": w, "site_points": 1, "blank_positions": blanks} for w, blanks in words]
        return {"dictionary": "nwl23", "more_available": False, "words": entries}

    monkeypatch.setattr(word_lookup, "lookup_words", lookup)


def test_the_layout_matches_the_screenshot_and_is_symmetrical():
    assert len(board.PREMIUMS) == 15
    assert all(len(row) == 15 for row in board.PREMIUMS)
    assert board.PREMIUMS == board.PREMIUMS[::-1]
    assert all(row == row[::-1] for row in board.PREMIUMS)
    # Read off the screenshot: corners are 3L, D1 and A4 are 3W, B2 and H4 are 2W, centre is plain.
    assert board.PREMIUMS[0][0] == "t"
    assert board.PREMIUMS[0][3] == board.PREMIUMS[3][0] == "T"
    assert board.PREMIUMS[1][1] == board.PREMIUMS[3][7] == "D"
    assert board.PREMIUMS[7][7] == "."
    assert board.PREMIUMS[7][3] == "D"
    assert board.PREMIUMS[0][7] == "d"


def test_tile_values_reproduce_the_sites_points_in_recorded_responses():
    fixtures = Path(__file__).parent / "fixtures"
    checked = 0
    for name in ("wordtips_helllo.json", "wordtips_qi_blank.json"):
        data = json.loads((fixtures / name).read_text())
        for page in word_lookup._word_pages(data):
            for entry in page["word_list"]:
                total = sum(
                    board.TILE_VALUES[letter.upper()]
                    for at, letter in enumerate(entry["word"])
                    if at not in entry["wildcards"]
                )
                assert total == entry["points"], entry["word"]
                checked += 1
    assert checked > 30


def test_a_letter_multiplier_counts_only_for_a_new_tile(monkeypatch):
    _site_listing(monkeypatch, ("CATS", []), ("CAT", []))

    # On an empty board, CAT at 8H puts T on J8 (2L): 3 + 1 + 1x2.
    assert _scored(EMPTY, "CAT", "CAT", "8H")["play_score"] == 6
    assert _scored(EMPTY, "CAT", "CAT", "8H")["premiums_used"] == ["2L"]
    # S on J8 (2L) is new, and C, A and T on the board count at face value: 3 + 1 + 1 + 1x2.
    assert _scored(CAT, "S", "CATS", "8G")["play_score"] == 7


def test_a_word_multiplier_doubles_the_word(monkeypatch):
    _site_listing(monkeypatch, ("HOTELS", []))

    # C8 to H8: H3, O on D8 (2W) 1, T1, E on F8 (2L) 2, L2, S1 = 10, doubled.
    found = _scored(EMPTY, "HOTELS", "HOTELS", "8C")
    assert found["play_score"] == 20
    assert found["premiums_used"] == ["2W", "2L"]


def test_using_all_seven_tiles_adds_the_bonus_after_multipliers(monkeypatch):
    _site_listing(monkeypatch, ("RETAINS", []))

    # B8 to H8: R1, E1, T on D8 (2W) 1, A1, I on F8 (2L) 2, N1, S1 = 8, doubled, then 40.
    assert _scored(EMPTY, "RETAINS", "RETAINS", "8B")["play_score"] == 56


def test_a_multiplier_also_applies_to_the_cross_word(monkeypatch):
    _site_listing(monkeypatch, ("SO", []))
    rows = list(EMPTY)
    rows[6] = "...A..........."

    # S on D8 (2W): SO = (1 + 1) x 2, and the cross-word AS = (1 + 1) x 2.
    found = _scored(rows, "SO", "SO", "8D")
    assert found["cross_words"] == ["AS"]
    assert found["play_score"] == 8


def test_plain_cross_words_add_their_face_value(monkeypatch):
    _site_listing(monkeypatch, ("ES", []))

    # ES at 9H is 2, plus AE (2) and TS (2).
    assert _scored(CAT, "ES", "ES", "9H")["play_score"] == 6


def test_a_blank_scores_nothing_even_on_a_multiplier(monkeypatch):
    _site_listing(monkeypatch, ("CAT", [2]))

    # T is the blank, on J8 (2L): 3 + 1 + 0.
    assert _scored(EMPTY, "CA?", "CAT", "8H")["play_score"] == 4


def test_placements_are_ranked_by_score_not_by_word_points(monkeypatch):
    def lookup(rack, letters):
        entries = [
            {"word": "HOTELS", "site_points": 10, "blank_positions": []},
            {"word": "QI", "site_points": 11, "blank_positions": []},
        ]
        return {"dictionary": "nwl23", "more_available": False, "words": entries}

    monkeypatch.setattr(word_lookup, "lookup_words", lookup)

    assert placements(EMPTY, "HOTELSQ" + "")[0]["word"] == "HOTELS"


@pytest.mark.parametrize("rows", [[], ["." * 15] * 14, ["." * 14] * 15, ["?" * 15] * 15])
def test_rejects_a_malformed_board(rows):
    with pytest.raises(WordLookupError):
        parse_board(rows)


def test_word_points_finds_only_the_exact_word(monkeypatch):
    pages = [
        {
            "num_words": 2,
            "num_pages": 1,
            "word_list": [
                {"word": "cat", "points": 5, "wildcards": []},
                {"word": "at", "points": 2, "wildcards": []},
            ],
        }
    ]
    monkeypatch.setattr(word_lookup, "_fetch", lambda word, client: {})
    monkeypatch.setattr(word_lookup, "_word_pages", lambda response: pages)

    assert word_lookup.word_points("CAT") == 5
    assert word_lookup.word_points("TAC") is None
    with pytest.raises(WordLookupError):
        word_lookup.word_points("C4T")
