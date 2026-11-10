import base64
import json

import anthropic
import httpx
import pytest
from fastapi.testclient import TestClient

from rackwise_coach import board, board_reader, coaching, main
from rackwise_coach.board_reader import BoardImageError, BoardReadError, read_board

PNG = base64.b64encode(b"\x89PNG\r\n\x1a\n" + b"data").decode()
ROWS = ["." * 15] * 7 + ["......CAT......"] + ["." * 15] * 7


def client_replying(reading, seen=None, stop_reason="end_turn"):
    def handler(request):
        if seen is not None:
            seen.append(json.loads(request.content))
        content = [{"type": "text", "text": json.dumps(reading)}] if reading else []
        body = {
            "id": "msg_1",
            "type": "message",
            "role": "assistant",
            "model": "claude-opus-5-5",
            "content": content,
            "stop_reason": stop_reason,
            "stop_sequence": None,
            "usage": {"input_tokens": 1, "output_tokens": 1},
        }
        return httpx.Response(200, json=body)

    return anthropic.Anthropic(
        api_key="test",
        max_retries=0,
        http_client=httpx.Client(transport=httpx.MockTransport(handler)),
    )


def test_reads_rows_and_rack_and_sends_the_image_with_the_layout(caplog):
    seen = []
    reading = {"is_board": True, "rows": ROWS, "rack": "fviwte?"}

    with caplog.at_level("INFO"):
        result = read_board(PNG, client_replying(reading, seen))

    assert "board reading used 1 input and 1 output tokens" in caplog.text
    assert PNG not in caplog.text

    assert result == {"rows": ROWS, "rack": "FVIWTE?"}
    body = seen[0]
    image, text = body["messages"][0]["content"]
    assert image["source"] == {"type": "base64", "media_type": "image/png", "data": PNG}
    assert all(row in text["text"] for row in board.PREMIUMS)
    assert body["system"] == coaching.load_prompt(board_reader.PROMPT_VERSION)
    assert body["output_config"]["effort"] == "high"
    assert body["output_config"]["format"]["type"] == "json_schema"
    assert body["fallbacks"] == "default"


@pytest.mark.parametrize(
    ("data", "media_type"),
    [
        (b"\xff\xd8\xff\xe0x", "image/jpeg"),
        (b"GIF89ax", "image/gif"),
        (b"RIFF\x00\x00\x00\x00WEBPx", "image/webp"),
    ],
)
def test_the_image_type_comes_from_its_bytes(data, media_type):
    seen = []
    encoded = base64.b64encode(data).decode()

    read_board(encoded, client_replying({"is_board": True, "rows": ROWS, "rack": ""}, seen))

    assert seen[0]["messages"][0]["content"][0]["source"]["media_type"] == media_type


@pytest.mark.parametrize(
    ("encoded", "reason"),
    [
        ("not base64!", "couldn't be read"),
        (base64.b64encode(b"just text").decode(), "PNG, JPEG"),
        (base64.b64encode(b"\x89PNG\r\n\x1a\n" + b"x" * (5 * 1024 * 1024)).decode(), "too large"),
    ],
)
def test_a_bad_upload_is_rejected_before_calling_claude(encoded, reason):
    seen = []
    with pytest.raises(BoardImageError, match=reason):
        read_board(encoded, client_replying({}, seen))
    assert seen == []


def test_a_picture_of_something_else_is_reported():
    with pytest.raises(BoardImageError, match="doesn't look like"):
        read_board(PNG, client_replying({"is_board": False, "rows": [], "rack": ""}))


@pytest.mark.parametrize(
    "reading",
    [
        {"is_board": True, "rows": ROWS[:14], "rack": ""},
        {"is_board": True, "rows": ["." * 14] * 15, "rack": ""},
        {"is_board": True, "rows": ["3" * 15] * 15, "rack": ""},
        {"is_board": True, "rows": ROWS, "rack": "ABCDEFGH"},
        {"is_board": True, "rows": ROWS, "rack": "A1"},
    ],
)
def test_an_impossible_reading_is_rejected(reading):
    with pytest.raises(BoardReadError):
        read_board(PNG, client_replying(reading))


def test_a_refusal_is_reported():
    with pytest.raises(BoardReadError):
        read_board(PNG, client_replying(None, stop_reason="refusal"))


@pytest.fixture
def api(monkeypatch):
    monkeypatch.setenv("ANTHROPIC_API_KEY", "test")
    return TestClient(main.app)


def post(api, data=PNG):
    return api.post("/v1/practice/read-board", json={"image_base64": data})


def test_the_endpoint_needs_credentials(monkeypatch):
    monkeypatch.delenv("ANTHROPIC_API_KEY", raising=False)
    monkeypatch.delenv("ANTHROPIC_AUTH_TOKEN", raising=False)

    response = post(TestClient(main.app))

    assert response.status_code == 503
    assert "ANTHROPIC_API_KEY" in response.json()["error"]


def test_the_endpoint_returns_a_reading(api, monkeypatch):
    monkeypatch.setattr(board_reader, "read_board", lambda image: {"rows": ROWS, "rack": "AB"})

    assert post(api).json() == {"rows": ROWS, "rack": "AB"}


@pytest.mark.parametrize(
    ("error", "status"),
    [(BoardImageError("bad picture"), 422), (BoardReadError("secret detail"), 502)],
)
def test_the_endpoint_reports_failures(api, monkeypatch, error, status):
    def fail(image):
        raise error

    monkeypatch.setattr(board_reader, "read_board", fail)

    response = post(api)

    assert response.status_code == status
    assert "secret detail" not in response.json()["error"]
