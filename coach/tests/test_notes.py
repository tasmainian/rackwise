import os
import re

import numpy as np
import pytest

from rackwise_coach import analysis, coaching, notes
from rackwise_coach.coaching import GameCoaching, check_coaching

VOCABULARY = 64


class FakeEmbedder:
    """A bag of words hashed into a small vector: deterministic, and similar words match."""

    model_name = "fake-model"

    def embed(self, texts):
        vectors = np.zeros((len(texts), VOCABULARY))
        for row, text in enumerate(texts):
            for word in re.findall(r"[a-z]+", text.lower()):
                vectors[row, sum(map(ord, word)) % VOCABULARY] += 1
        return vectors + 1e-6


def note(note_id, tags, summary="A summary.", title="A title"):
    return notes.Note(note_id, title, tuple(tags), summary, "The body.")


def option(word, score, tiles, leave, whole=False, premiums=(), **facts):
    return {
        "word": word,
        "position": "8H",
        "words_formed": [word],
        "play_score": score,
        "premiums_used": list(premiums),
        "rack_tiles_used": tiles,
        "uses_whole_rack": whole,
        "leave": leave,
        "leave_facts": {
            "vowels": facts.get("vowels", 1),
            "consonants": facts.get("consonants", 2),
            "blanks": facts.get("blanks", 0),
            "duplicate_letters": facts.get("duplicate_letters", []),
            "q_without_u": facts.get("q_without_u", False),
        },
        # Fields the prompt doesn't use, which build_input should drop.
        "site_points": score,
        "blank_positions": [],
        "direction": "across",
        "cross_words": [],
    }


def a_turn(played, options, number=1, status="listed", behind=None):
    if played is not None:
        best = max([o["play_score"] for o in options] + [played["play_score"]])
        played = played | {
            "status": status,
            "invalid_words": [] if status == "listed" else [played["word"]],
            "points_behind_best": best - played["play_score"] if behind is None else behind,
        }
    return {
        "turn": number,
        "rack": "ABCDEFG",
        "more_available": False,
        "options": options,
        "played": played,
    }


# Note files


def test_every_note_file_is_valid_and_every_tag_has_a_note():
    library = notes.load_notes()

    assert len(library) == 50
    assert {tag for n in library for tag in n.tags} == notes.TAGS


@pytest.mark.parametrize(
    ("text", "problem"),
    [
        ("no header", "header"),
        ("---\nid: N01\ntitle: T\ntags: duplicates\n---\nBody", "summary"),
        ("---\nid: X1\ntitle: T\ntags: duplicates\nsummary: S\n---\nBody", "ID"),
        ("---\nid: N01\ntitle: T\ntags: defence\nsummary: S\n---\nBody", "tags"),
        ("---\nid: N01\ntitle: T\ntags: duplicates\nsummary: Keep 2 S's\n---\nBody", "digits"),
        ("---\nid: N01\ntitle: Keep QI\ntags: duplicates\nsummary: S\n---\nBody", "digits"),
        ("---\nid: N01\ntitle: T\ntags: duplicates\nsummary: S\n---\n", "body"),
    ],
)
def test_a_malformed_note_is_rejected(text, problem):
    with pytest.raises(notes.NoteError, match=problem):
        notes.parse_note(text)


def test_a_note_parses_its_fields():
    header = "id: N07\ntitle: Keep an E\ntags: vowel_heavy, duplicates\nsummary: E helps."
    text = f"---\n{header}\n---\nWhy."
    parsed = notes.parse_note(text)

    assert parsed == notes.Note(
        "N07", "Keep an E", ("vowel_heavy", "duplicates"), "E helps.", "Why."
    )
    assert parsed.brief() == {"id": "N07", "title": "Keep an E", "summary": "E helps."}


# Tagging turns


def test_a_play_close_to_the_best_with_a_fine_leave_has_no_lesson():
    played = option("CAB", 7, "ABC", "DEFG")
    assert analysis.mistake_tags(a_turn(played, [option("FACED", 10, "ACDEF", "BG"), played])) == []


def test_a_swap_is_tagged_and_a_missed_seven_tile_play_comes_first():
    bingo = option("TRAINED", 47, "ADEINRT", "", whole=True)

    assert analysis.mistake_tags(a_turn(None, [])) == ["swap"]
    assert analysis.mistake_tags(a_turn(None, [bingo])) == ["missed_bingo", "swap"]


def test_an_invalid_play_is_tagged_as_that_alone():
    played = option("ZQX", 30, "QXZ", "ABCD")
    assert analysis.mistake_tags(a_turn(played, [], status="not_valid")) == ["invalid_play"]


def test_a_big_gap_is_tagged_with_what_the_best_play_did_differently():
    best = option("QUITERS", 57, "EIQRSTU", "", whole=True, premiums=["3W"])
    played = option("SUITE", 5, "EISTU", "QR", q_without_u=True, vowels=0)

    tags = analysis.mistake_tags(a_turn(played, [best, played]))

    assert tags == ["missed_bingo", "premium_missed", "q_without_u", "s_spent", "low_score"]


def test_a_poor_leave_counts_only_when_a_close_play_avoided_it():
    played = option("TEND", 5, "DENT", "GUU", duplicate_letters=["U"])
    close = option("NUDGE", 7, "DEGNU", "TU")
    far = option("NUDGE", 4, "DEGNU", "TU")

    assert analysis.mistake_tags(a_turn(played, [close, played])) == ["duplicates"]
    assert analysis.mistake_tags(a_turn(played, [far, played])) == ["duplicates"]
    # The only play without the duplicate scored far less, so keeping it was the price of points.
    big = option("TEND", 40, "DENT", "GUU", duplicate_letters=["U"])
    assert analysis.mistake_tags(a_turn(big, [option("NUDGE", 30, "DEGNU", "TU"), big])) == []


def test_spending_the_blank_cheaply_is_tagged_but_a_balanced_blank_leave_is_not_heavy():
    played = option("CAT", 2, "?AT", "ELRS", vowels=1, consonants=3)
    staler = option("STALER", 6, "AELRST", "?", vowels=0, consonants=0, blanks=1)

    assert analysis.mistake_tags(a_turn(played, [staler, played])) == [
        "blank_spent",
        "consonant_heavy",
    ]
    kept = option("CAT", 2, "ACT", "?LRS", vowels=0, consonants=3, blanks=1)
    assert "consonant_heavy" not in analysis.mistake_tags(a_turn(kept, [staler, kept]))


def test_a_query_describes_the_turns_details():
    played = option("TEND", 5, "DENT", "EUU", duplicate_letters=["U"], vowels=3, consonants=0)
    turn = a_turn(played, [option("NUDGE", 7, "DEGNU", "TU"), played])

    assert "Kept two U's, a doubled vowel." in analysis.mistake_query(turn, "duplicates")
    assert "Kept 3 vowels and 0 consonants." in analysis.mistake_query(turn, "vowel_heavy")
    assert "No play was found." in analysis.mistake_query(a_turn(None, []), "swap")


# Retrieval


def library():
    return [
        note("N01", ["duplicates"], "Two of a letter cut your words."),
        note("N02", ["duplicates", "vowel_heavy"], "Doubled vowels are the worst pair."),
        note("N03", ["vowel_heavy"], "Too many vowels."),
        note("N04", ["q_without_u"], "A Q with no U is hard to play."),
        note("N05", ["low_score"], "Weigh points against the leave."),
    ]


def test_retrieval_takes_one_note_per_tag_in_order():
    embedder = FakeEmbedder()
    store = notes.MemoryStore(library(), embedder)
    queries = {"q_without_u": "Kept a Q with no U.", "duplicates": "Kept doubled vowels."}

    found = notes.retrieve(store, embedder, queries)

    assert [n["id"] for n in found] == ["N04", "N02"]
    assert found[0] == {
        "id": "N04",
        "title": "A title",
        "summary": "A Q with no U is hard to play.",
    }


def test_retrieval_skips_a_note_already_chosen_for_an_earlier_tag():
    embedder = FakeEmbedder()
    store = notes.MemoryStore(library(), embedder)
    queries = {"duplicates": "Doubled vowels.", "vowel_heavy": "Doubled vowels."}

    assert [n["id"] for n in notes.retrieve(store, embedder, queries)] == ["N02", "N03"]


def test_retrieval_stops_at_k_and_falls_back_when_no_note_has_the_tags():
    embedder = FakeEmbedder()
    store = notes.MemoryStore(library(), embedder)
    many = {"q_without_u": "Q", "duplicates": "pair", "vowel_heavy": "vowels", "low_score": "x"}

    assert len(notes.retrieve(store, embedder, many, k=2)) == 2
    assert len(notes.retrieve(store, embedder, {"swap": "Swapped."}, k=2)) == 2


def test_only_mistake_turns_get_notes():
    embedder = FakeEmbedder()
    store = notes.MemoryStore(library(), embedder)
    good = option("CAB", 7, "ABC", "DEFG")
    bad = option("TEND", 5, "DENT", "GUU", duplicate_letters=["U"])
    turns = [a_turn(good, [good]), a_turn(bad, [option("NUDGE", 7, "DEGNU", "TU"), bad], number=2)]

    found = notes.notes_for_turns(turns, store, embedder)

    assert list(found) == [2]
    assert found[2][0]["id"] in {"N01", "N02"}


# What Claude is sent


def test_build_input_trims_fields_and_options_and_sends_each_note_once():
    good = option("CAB", 7, "ABC", "DEFG")
    bad = option("TEND", 5, "DENT", "GUU", duplicate_letters=["U"])
    others = [option(f"W{i}", 1, "A", "BCDEFG") for i in range(6)]
    turns = [a_turn(good, [good, *others]), a_turn(bad, [bad, *others], number=2, behind=10)]
    game = {"stats": analysis.game_stats(turns), "turns": turns}
    brief = {"id": "N01", "title": "T", "summary": "S"}

    sent = coaching.build_input(game, {1: [brief], 2: [brief]})

    assert len(sent["turns"][0]["options"]) == coaching.OPTIONS_OTHERWISE
    assert len(sent["turns"][1]["options"]) == coaching.OPTIONS_FOR_MISTAKES
    assert sent["turns"][1]["note_ids"] == ["N01"]
    assert sent["strategy_notes"] == [brief]
    for field in ("site_points", "blank_positions", "direction", "cross_words", "rack_tiles_used"):
        assert field not in sent["turns"][0]["options"][0]
        assert field not in sent["turns"][1]["played"]
    assert sent["turns"][1]["played"]["points_behind_best"] == 10


def test_build_input_without_notes_sends_no_note_fields():
    played = option("CAB", 7, "ABC", "DEFG")
    sent = coaching.build_input({"stats": {}, "turns": [a_turn(played, [played])]})

    assert "strategy_notes" not in sent
    assert "note_ids" not in sent["turns"][0]


def coached(cited, note_text="Two U's are hard to keep."):
    return GameCoaching(
        summary="Fine.",
        themes=[],
        turns=[{"turn": 1, "recommended_word": None, "note": note_text, "cited_notes": cited}],
    )


def sent_with_note():
    played = option("TEND", 5, "DENT", "GUU", duplicate_letters=["U"])
    game = {"stats": {}, "turns": [a_turn(played, [played])]}
    return coaching.build_input(game, {1: [{"id": "N01", "title": "T", "summary": "S"}]})


def test_the_check_allows_citing_a_sent_note_and_naming_it():
    assert check_coaching(coached(["N01"], "See N01 on keeping two U's."), sent_with_note()) == []


def test_the_check_rejects_a_note_that_wasnt_sent_for_that_turn():
    problems = check_coaching(coached(["N09"]), sent_with_note())

    assert problems == ["cited N09, which wasn't sent for turn 1"]


def test_an_unknown_effort_is_refused(monkeypatch):
    monkeypatch.setenv("COACH_EFFORT", "extreme")

    with pytest.raises(coaching.CoachError, match="COACH_EFFORT"):
        coaching.coach_game({"stats": {}, "turns": []}, client=object())


# Real Postgres with pgvector, as in the engine's session tests.

DATABASE_URL = os.environ.get("TEST_DATABASE_URL")


@pytest.fixture
def postgres():
    if not DATABASE_URL:
        pytest.skip("set TEST_DATABASE_URL to test against real Postgres")
    conn = notes.connect(DATABASE_URL)
    # A throwaway schema, so the real notes table is never touched.
    conn.run("DROP SCHEMA IF EXISTS rackwise_test CASCADE")
    conn.run("CREATE SCHEMA rackwise_test")
    conn.run("SET search_path TO rackwise_test, public")
    yield conn
    conn.run("DROP SCHEMA rackwise_test CASCADE")
    conn.close()


def test_postgres_store_embeds_only_changed_notes_and_filters_by_tag(postgres):
    embedder = FakeEmbedder()
    store = notes.PostgresStore(postgres)

    assert store.sync(library(), embedder) == 5
    assert store.sync(library(), embedder) == 0
    edited = [note("N01", ["duplicates"], "Pairs block seven-tile plays."), *library()[1:4]]
    assert store.sync(edited, embedder) == 1

    vector = embedder.embed(["Kept doubled vowels."])[0]
    assert [n["id"] for n in store.search(["vowel_heavy"], vector, 3)] in (
        ["N02", "N03"],
        ["N03", "N02"],
    )
    assert {n["id"] for n in store.search(None, vector, 10)} == {"N01", "N02", "N03", "N04"}
    assert store.bodies(["N04", "N05"]) == {"N04": {"title": "A title", "body": "The body."}}

    found = notes.retrieve(store, embedder, {"q_without_u": "Kept a Q.", "duplicates": "pair"})
    assert [n["id"] for n in found][0] == "N04"
