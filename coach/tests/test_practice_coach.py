import importlib.util
import json
from pathlib import Path

import anthropic
import httpx
import pytest
from fastapi.testclient import TestClient

from rackwise_coach import analysis, coaching, main, word_lookup
from rackwise_coach.coaching import CoachError, CoachRejected, GameCoaching, check_coaching

EVALS = Path(__file__).resolve().parents[2] / "evals"
spec = importlib.util.spec_from_file_location("eval_score", EVALS / "score.py")
eval_score = importlib.util.module_from_spec(spec)
spec.loader.exec_module(eval_score)

EMPTY = ["." * 15] * 15
CAT = ["." * 15] * 7 + ["......CAT......"] + ["." * 15] * 7


def placed(word, score, tiles, cross=(), position="8H"):
    return {
        "word": word,
        "site_points": score,
        "blank_positions": [],
        "direction": "across",
        "position": position,
        "rack_tiles_used": tiles,
        "cross_words": [{"word": w, "site_points": p} for w, p in cross],
        "play_score": score + sum(p for _, p in cross),
        "premiums_used": [],
    }


NUDGE = placed("NUDGE", 7, "DEGNU")
TEND = placed("TEND", 5, "DENT", position="H8")


def turn(played=TEND, options=(NUDGE, TEND), number=1, status="listed"):
    described = played | {"status": status, "invalid_words": []} if played else None
    return analysis.compare("UUDGENT", list(options), described) | {"turn": number}


def game(*turns):
    turns = turns or (turn(),)
    return {"stats": analysis.game_stats(list(turns)), "turns": list(turns)}


def answer(recommended="NUDGE", summary="NUDGE was better.", note="TEND keeps GUU.", number=1):
    return GameCoaching(
        summary=summary,
        themes=[],
        turns=[{"turn": number, "recommended_word": recommended, "note": note}],
    )


def test_compare_reports_points_behind_the_leave_and_every_word_formed():
    played = placed("ES", 2, "ES", cross=[("AE", 2), ("TS", 2)]) | {"status": "listed"}
    result = analysis.compare("UUDGENTS", [placed("NUDGE", 9, "DEGNU")], played)["played"]

    assert result["words_formed"] == ["ES", "AE", "TS"]
    assert result["play_score"] == 6
    assert result["points_behind_best"] == 3


def test_compare_reports_the_leave_facts():
    result = turn()

    assert result["played"]["leave"] == "GUU"
    assert result["played"]["leave_facts"]["duplicate_letters"] == ["U"]
    assert result["options"][0]["leave"] == "TU"


def test_a_play_better_than_anything_the_search_found_is_zero_behind():
    played = placed("ZEBRA", 30, "ABERZ") | {"status": "listed"}
    result = analysis.compare("ZEBRATS", [placed("AT", 2, "AT")], played)["played"]

    assert result["points_behind_best"] == 0


def test_an_invalid_play_has_no_points_behind():
    played = placed("TSE", 3, "EST") | {"status": "not_valid", "invalid_words": ["TSE"]}

    assert (
        analysis.compare("TSE", [placed("SET", 3, "EST")], played)["played"]["points_behind_best"]
        is None
    )


def test_whole_rack_and_q_without_u_are_flagged():
    whole = analysis.compare("TRAINED", [placed("TRAINED", 7, "ADEINRT")])["options"][0]
    q_kept = analysis.compare("QAT", [placed("AT", 2, "AT")])["options"][0]

    assert whole["uses_whole_rack"] is True
    assert q_kept["leave_facts"]["q_without_u"] is True


def test_game_stats_add_up_the_turns():
    good = turn(number=1)
    best = turn(played=NUDGE, number=2)
    passed = turn(played=None, number=3)
    bad = turn(number=4, status="not_valid")

    stats = analysis.game_stats([good, best, passed, bad])

    assert stats["turns"] == 4
    assert stats["swaps_or_passes"] == 1
    assert stats["invalid_plays"] == 1
    assert stats["total_score"] == 12
    assert stats["total_points_behind"] == 2
    assert stats["average_points_behind"] == 1.0
    assert stats["biggest_miss_points"] == 2
    assert stats["biggest_miss_turn"] == 1


def test_game_stats_handle_a_game_with_no_plays():
    stats = analysis.game_stats([turn(played=None)])

    assert stats["average_points_behind"] == 0
    assert stats["biggest_miss_turn"] is None


@pytest.mark.parametrize(
    ("kwargs", "expected"),
    [
        ({"recommended": "ZEBRA"}, "recommended ZEBRA"),
        ({"note": "ZEBRA would be better."}, "mentioned ZEBRA"),
        ({"note": "That costs 40 points."}, "mentioned 40"),
        ({"number": 9}, "turn 9"),
    ],
)
def test_check_catches_anything_outside_the_data(kwargs, expected):
    problems = check_coaching(answer(**kwargs), game())

    assert any(expected in problem for problem in problems)


def test_check_accepts_stats_positions_multipliers_and_turn_numbers():
    text = "On turn 1 NUDGE beats TEND at H8 by 2, on average 2.0 behind, without any 3W."

    assert check_coaching(answer(note=text), game()) == []


def test_check_allows_the_rack_without_its_blank_but_not_other_letters():
    data = game(analysis.compare("?AELRST", [placed("STALER", 6, "AELRST")]) | {"turn": 1})

    assert check_coaching(answer("STALER", summary="AELRST plus a blank.", note="Yes."), data) == []
    problems = check_coaching(answer("STALER", summary="ALERTS would do.", note="Yes."), data)
    assert any("ALERTS" in problem for problem in problems)


def test_check_catches_an_invented_position():
    problems = check_coaching(answer(note="Play it at O15."), game())

    assert any("O15" in problem for problem in problems)


def test_a_recommendation_must_belong_to_that_turn():
    other = turn(options=(placed("ZEBRA", 9, "DEGNU"),), number=2)

    problems = check_coaching(answer(recommended="ZEBRA"), game(turn(), other))

    assert any("recommended ZEBRA" in problem for problem in problems)


def test_an_invalid_play_cant_be_recommended():
    bad = turn(status="not_valid", options=(NUDGE,))

    assert check_coaching(answer(recommended="TEND"), game(bad))


def anthropic_client(payload, seen=None):
    def handler(request):
        if seen is not None:
            seen.append(json.loads(request.content))
        return httpx.Response(200, json=payload)

    return anthropic.Anthropic(
        api_key="test",
        max_retries=0,
        http_client=httpx.Client(transport=httpx.MockTransport(handler)),
    )


def message(text=None, stop_reason="end_turn"):
    return {
        "id": "msg_1",
        "type": "message",
        "role": "assistant",
        "model": "claude-opus-5-5",
        "content": [{"type": "text", "text": text}] if text else [],
        "stop_reason": stop_reason,
        "stop_sequence": None,
        "usage": {"input_tokens": 1, "output_tokens": 1},
    }


def test_one_call_sends_the_prompt_file_and_the_whole_game():
    seen = []
    client = anthropic_client(message(answer().model_dump_json()), seen=seen)

    result = coaching.coach_game(game(), client)

    assert result.turns[0].recommended_word == "NUDGE"
    assert len(seen) == 1
    body = seen[0]
    assert body["model"] == "claude-sonnet-5-5"
    assert body["fallbacks"] == "default"
    assert body["output_config"]["effort"] == "medium"
    assert body["output_config"]["format"]["type"] == "json_schema"
    assert body["system"] == coaching.load_prompt()
    assert json.loads(body["messages"][0]["content"])["turns"][0]["played"]["word"] == "TEND"


def test_coach_withholds_an_answer_with_an_invented_word():
    bad = answer(note="ZEBRA is better.").model_dump_json()
    with pytest.raises(CoachRejected):
        coaching.coach_game(game(), anthropic_client(message(bad)))


def test_coach_reports_a_refusal():
    with pytest.raises(CoachError):
        coaching.coach_game(game(), anthropic_client(message(stop_reason="refusal")))


@pytest.fixture
def api(monkeypatch):
    def lookup(rack, letters):
        entries = [
            {"word": "NUDGE", "site_points": 7, "blank_positions": []},
            {"word": "TEND", "site_points": 5, "blank_positions": []},
        ]
        return {"dictionary": "nwl23", "more_available": False, "words": entries}

    monkeypatch.setattr(word_lookup, "lookup_words", lookup)
    monkeypatch.setattr(word_lookup, "word_points", {"TEND": 5, "NUDGE": 7}.get)
    monkeypatch.delenv("ANTHROPIC_API_KEY", raising=False)
    monkeypatch.delenv("ANTHROPIC_AUTH_TOKEN", raising=False)
    return TestClient(main.app)


def tiles(word, row=7, col=7):
    return [{"row": row, "col": col + i, "letter": letter} for i, letter in enumerate(word)]


def test_check_play_scores_a_stack_without_any_lookup(monkeypatch):
    def fail(*args):
        raise AssertionError("recording a play must not call the site")

    monkeypatch.setattr(word_lookup, "lookup_words", fail)
    monkeypatch.setattr(word_lookup, "word_points", fail)
    body = {"rack": "ES", "board": CAT, "tiles": tiles("ES", row=8)}

    response = TestClient(main.app).post("/v1/practice/check-play", json=body).json()

    assert response == {"words_formed": ["ES", "AE", "TS"], "position": "9H", "play_score": 6}


def test_check_play_explains_an_illegal_play():
    body = {"rack": "ES", "board": CAT, "tiles": tiles("ES", row=0, col=0)}

    response = TestClient(main.app).post("/v1/practice/check-play", json=body)

    assert response.status_code == 422
    assert "connect" in response.json()["error"]


def test_review_compares_a_placed_play_with_the_best_found(api):
    # TEND at 8H: T1, E1, N on J8 (2L) 2, D2 = 6. NUDGE at 8D doubles 12 on the 2W.
    body = {"rack": "UUDGENT", "board": EMPTY, "tiles": tiles("TEND")}

    result = api.post("/v1/practice/review", json=body).json()

    assert result["played"]["status"] == "listed"
    assert result["played"]["position"] == "8H"
    assert result["played"]["play_score"] == 6
    assert result["played"]["leave"] == "GUU"
    assert result["options"][0]["play_score"] == 24
    assert result["played"]["points_behind_best"] == 18


def test_review_reports_a_placed_word_the_site_rejects(api):
    body = {"rack": "UUDGENT", "board": EMPTY, "tiles": tiles("TUG")}

    played = api.post("/v1/practice/review", json=body).json()["played"]

    assert played["status"] == "not_valid"
    assert played["invalid_words"] == ["TUG"]
    assert played["points_behind_best"] is None


def test_review_of_a_swap_or_pass_still_lists_the_options(api):
    result = api.post("/v1/practice/review", json={"rack": "UUDGENT", "board": EMPTY}).json()

    assert result["played"] is None
    assert result["options"]


def test_review_reports_bad_input_and_site_failures(monkeypatch):
    client = TestClient(main.app)
    for body in ({"rack": "H1", "board": EMPTY}, {"rack": "HE", "board": ["."]}):
        assert client.post("/v1/practice/review", json=body).status_code == 422

    def down(rack, letters):
        raise word_lookup.WordSiteError("word.tips request failed: 503")

    monkeypatch.setattr(word_lookup, "lookup_words", down)
    response = client.post("/v1/practice/review", json={"rack": "HE", "board": EMPTY})
    assert response.status_code == 502
    assert "word.tips" in response.json()["error"]


def test_summary_returns_stats_without_an_api_key(api):
    response = api.post("/v1/practice/summary", json={"turns": [turn()]}).json()

    assert response["coach_status"] == "no_api_key"
    assert response["coaching"] is None
    assert response["stats"]["total_points_behind"] == 2


def test_summary_returns_coaching(api, monkeypatch):
    monkeypatch.setenv("ANTHROPIC_API_KEY", "test")
    monkeypatch.setattr(coaching, "coach_game", lambda data: answer())

    response = api.post("/v1/practice/summary", json={"turns": [turn()]}).json()

    assert response["coach_status"] == "ok"
    assert response["coaching"]["turns"][0]["recommended_word"] == "NUDGE"


def test_summary_sends_notes_and_returns_the_full_text_of_cited_ones(api, monkeypatch):
    brief = {"id": "N01", "title": "Pairs", "summary": "Two of a letter."}
    bodies = {"N01": {"title": "Pairs", "body": "Long."}, "N02": {"title": "B", "body": "B."}}
    sent = {}

    def coach(data):
        sent.update(data)
        cited = answer()
        cited.turns[0].cited_notes = ["N01"]
        return cited

    monkeypatch.setenv("ANTHROPIC_API_KEY", "test")
    monkeypatch.setattr(main, "_strategy_notes", lambda turns: ({1: [brief]}, bodies))
    monkeypatch.setattr(coaching, "coach_game", coach)

    response = api.post("/v1/practice/summary", json={"turns": [turn()]}).json()

    assert sent["strategy_notes"] == [brief]
    assert sent["turns"][0]["note_ids"] == ["N01"]
    assert response["notes"] == {"N01": {"title": "Pairs", "body": "Long."}}


def test_summary_coaches_without_notes_when_there_is_no_database(api, monkeypatch):
    monkeypatch.setenv("ANTHROPIC_API_KEY", "test")
    monkeypatch.delenv("DATABASE_URL", raising=False)
    monkeypatch.setattr(coaching, "coach_game", lambda data: answer())

    response = api.post("/v1/practice/summary", json={"turns": [turn()]}).json()

    assert response["coach_status"] == "ok"
    assert response["notes"] == {}


@pytest.mark.parametrize(
    ("error", "status"),
    [(CoachRejected("no"), "rejected"), (CoachError("no"), "error")],
)
def test_summary_keeps_the_stats_when_coaching_fails(api, monkeypatch, error, status):
    def fail(data):
        raise error

    monkeypatch.setenv("ANTHROPIC_API_KEY", "test")
    monkeypatch.setattr(coaching, "coach_game", fail)

    response = api.post("/v1/practice/summary", json={"turns": [turn()]}).json()

    assert response["coach_status"] == status
    assert response["stats"]["turns"] == 1


def test_summary_rejects_a_malformed_turn(api):
    assert api.post("/v1/practice/summary", json={"turns": [{}]}).status_code == 422


def without_retrieval(position):
    """The case and its trimmed input, with no notes. `score.py --check` grades retrieval,
    because that needs the real embedding model."""
    case = {key: value for key, value in position.items() if key != "retrieves"}
    return case, coaching.build_input(eval_score.game_for(position))


def test_eval_set_has_17_distinct_cases_including_whole_games():
    positions = eval_score.load_positions()

    assert len({p["id"] for p in positions}) == 17
    assert sum("turns" in p for p in positions) == 2


def test_a_whole_game_case_numbers_its_turns_and_adds_them_up():
    case = next(p for p in eval_score.load_positions() if p["id"] == "habit-keeping-two-us")
    data = eval_score.game_for(case)

    assert [t["turn"] for t in data["turns"]] == [1, 2, 3]
    assert [t["played"]["leave"] for t in data["turns"]] == ["GUU", "EUU", "UU"]
    assert data["stats"]["total_points_behind"] == 2 + 2 + 1


def test_eval_scores_add_the_bonus_for_using_every_tile():
    case = next(p for p in eval_score.load_positions() if p["id"] == "missed-bingo")
    turn = eval_score.game_for(case)["turns"][0]

    assert turn["options"][0]["play_score"] == 8 + 40
    assert turn["played"]["points_behind_best"] == 48 - 7


def test_no_recommendation_counts_as_backing_the_players_own_play():
    case = next(p for p in eval_score.load_positions() if p["id"] == "good-choice")
    data = eval_score.game_for(case)
    backed = answer(recommended=None, summary="HOTELS was best.", note="Good.")

    assert eval_score.grade(case, backed, data) == []


@pytest.mark.parametrize("position", eval_score.load_positions(), ids=lambda p: p["id"])
def test_every_reference_answer_passes_the_grader(position):
    # Also proves each case's words can be made from its racks, and that trimming the input
    # keeps every word and number the reference answers use.
    position, data = without_retrieval(position)
    reference = GameCoaching.model_validate(position["reference"])

    assert eval_score.grade(position, reference, data) == []


@pytest.mark.parametrize(
    "position",
    [p for p in eval_score.load_positions() if "recommend" in p],
    ids=lambda p: p["id"],
)
def test_the_grader_fails_a_wrong_recommendation(position):
    position, data = without_retrieval(position)
    reference = GameCoaching.model_validate(position["reference"])
    reference.turns[0].recommended_word = "ZEBRA"

    assert eval_score.grade(position, reference, data)
