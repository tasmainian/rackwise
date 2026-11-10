"""Reads the tiles and rack from a picture of the player's game board, using Claude.

This only transcribes. Whatever it reads goes back onto the grid for the player to check,
and word validity and scoring stay with word.tips and board.py.
See docs/decisions/0009-board-from-picture.md.
"""

import base64
import binascii
import logging
import os
import re

import anthropic
from pydantic import BaseModel, ValidationError

from rackwise_coach import board
from rackwise_coach.coaching import DEFAULT_MODEL, load_prompt, log_usage

logger = logging.getLogger(__name__)
PROMPT_VERSION = "board-reader-v1"
# The Claude API's limit for one image.
MAX_IMAGE_BYTES = 5 * 1024 * 1024
_RACK = re.compile(r"[A-Z?]{0,7}")
_SIGNATURES = {
    b"\x89PNG\r\n\x1a\n": "image/png",
    b"\xff\xd8\xff": "image/jpeg",
    b"GIF87a": "image/gif",
    b"GIF89a": "image/gif",
}


class BoardReading(BaseModel):
    is_board: bool
    rows: list[str]
    rack: str


class BoardImageError(Exception):
    """The upload can't be used, so the player should choose another picture."""


class BoardReadError(Exception):
    """Claude couldn't give a usable reading of the picture."""


def read_board(image_base64: str, client: anthropic.Anthropic | None = None) -> dict:
    """Return the 15 board rows and the rack found in a picture."""
    media_type = _check_image(image_base64)
    client = client or anthropic.Anthropic()
    layout = "\n".join(board.PREMIUMS)
    response = client.beta.messages.create(
        model=os.environ.get("COACH_MODEL", DEFAULT_MODEL),
        max_tokens=16000,
        betas=["server-side-fallback-2026-07-01"],
        fallbacks="default",
        # Misplacing a tile by one square spoils the whole review, so this gets more effort.
        output_config={
            "effort": "high",
            "format": {"type": "json_schema", "schema": anthropic.transform_schema(BoardReading)},
        },
        system=load_prompt(PROMPT_VERSION),
        messages=[
            {
                "role": "user",
                "content": [
                    {
                        "type": "image",
                        "source": {
                            "type": "base64",
                            "media_type": media_type,
                            "data": image_base64,
                        },
                    },
                    {"type": "text", "text": f"The multiplier layout:\n{layout}"},
                ],
            }
        ],
    )
    log_usage(logger, "board reading", response)
    text = "".join(block.text for block in response.content if block.type == "text")
    try:
        reading = BoardReading.model_validate_json(text)
    except ValidationError as err:
        raise BoardReadError(f"no usable reading (stop reason: {response.stop_reason})") from err
    if not reading.is_board:
        raise BoardImageError("That picture doesn't look like a game board.")
    return _checked(reading)


def _check_image(image_base64: str) -> str:
    """Work out the type from the bytes, because a browser's label can't be relied on."""
    try:
        image = base64.b64decode(image_base64, validate=True)
    except (binascii.Error, ValueError) as err:
        raise BoardImageError("That file couldn't be read as an image.") from err
    if len(image) > MAX_IMAGE_BYTES:
        raise BoardImageError("That picture is too large. The limit is 5 MB.")
    if image[:4] == b"RIFF" and image[8:12] == b"WEBP":
        return "image/webp"
    for signature, media_type in _SIGNATURES.items():
        if image.startswith(signature):
            return media_type
    raise BoardImageError("Please upload a PNG, JPEG, GIF or WebP picture.")


def _checked(reading: BoardReading) -> dict:
    rack = reading.rack.upper()
    if not _RACK.fullmatch(rack):
        raise BoardReadError(f"read an impossible rack: {reading.rack}")
    try:
        rows = board.parse_board(reading.rows)
    except board.WordLookupError as err:
        raise BoardReadError(f"read an impossible board: {err}") from err
    return {"rows": rows, "rack": rack}
