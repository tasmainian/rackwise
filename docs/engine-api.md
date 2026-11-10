# Engine API

The engine is the only source of truth for whether a word is valid and what a play scores. The analysis endpoints are stateless, with each request carrying the position. The `/v1/games` endpoints run saved games between a person and a bot.

## Conventions

- **Board:** 15 strings of 15 characters. `.` is empty, an uppercase letter is a normal tile, and a lowercase letter is a played blank standing for that letter.
- **Rack:** up to 7 tiles, `A` to `Z`, with `?` for an unplayed blank.
- **Tiles:** `row` and `col` count from 0 at the top left. `letter` is lowercase for a blank.
- **Errors:** a malformed request returns 400 with `{"error": "..."}`. Request bodies are limited to 16 KiB.

## `GET /healthz`

Returns `{"status": "ok", "service": "engine"}`.

## `POST /v1/moves`

Returns every legal play, highest score first, with ties in a fixed order.

```json
{
  "board": ["...............", "... 15 rows ..."],
  "rack": "AEINRS?",
  "limit": 5,
  "bag_remaining": 40
}
```

`limit` is optional, and 0 or missing means no limit. `bag_remaining` is optional, and only affects `leave_value` and `equity`: when it is 0, the leave is valued by its tiles' points, counted against the player. If it's missing, the bag is treated as not empty.

```json
{
  "total": 1234,
  "moves": [
    {
      "row": 7,
      "col": 7,
      "direction": "across",
      "position": "8H",
      "word": "RETAINS",
      "tiles": [{ "row": 7, "col": 7, "letter": "R" }],
      "cross_words": [],
      "score": 56,
      "bingo": true,
      "leave": "",
      "leave_value": 0,
      "lanes_opened": 0,
      "equity": 56
    }
  ]
}
```

| Field | Meaning |
|---|---|
| `total` | Number of legal plays, even when `limit` shortens the list |
| `position` | GCG notation: row first for across (`8H`), column first for down (`H8`) |
| `word` | The whole main word, including tiles already on the board |
| `tiles` | Only the newly placed tiles |
| `leave` | Tiles kept, sorted |
| `leave_value` | Estimated points that keeping those tiles is worth |
| `lanes_opened` | New routes this play opens to empty triple-word squares |
| `equity` | `score + leave_value - lane penalty x lanes_opened` |

A single tile that forms words both ways is reported as across. Each play appears once.

## `POST /v1/validate`

Checks one proposed play. An illegal play is a normal answer, so it still returns 200.

```json
{
  "board": ["..."],
  "rack": "ZA",
  "tiles": [
    { "row": 7, "col": 7, "letter": "Z" },
    { "row": 7, "col": 8, "letter": "A" }
  ],
  "bag_remaining": 40
}
```

```json
{ "legal": false, "code": "invalid_word", "reason": "ZA is not a valid word" }
```

A legal play returns `{"legal": true, "move": { ... }}`, with `move` in the same form as above.

Codes: `no_tiles`, `invalid_letter`, `out_of_bounds`, `square_occupied`, `duplicate_square`, `not_in_line`, `gap_in_word`, `tile_not_on_rack`, `must_cover_center`, `not_connected`, `word_too_short`, `invalid_word`.

## `POST /v1/bot-move`

Asks a bot for its turn.

```json
{
  "board": ["..."],
  "rack": "AEINRST",
  "level": "hard",
  "bag_remaining": 40,
  "seed": 7
}
```

`level` is `easy`, `medium` or `hard`. `bag_remaining` is required, because exchanges and leave values depend on it. `seed` is optional.

```json
{ "kind": "play", "move": { "word": "RETAINS" }, "seed": 7 }
```

`kind` is `play` (with `move`), `exchange` (with `exchange`, the tiles to give up) or `pass`. `seed` is always returned, so a bot's choice can be replayed.

## `GET /v1/rules`

Returns the board layout and tile values, so that the frontend doesn't define them a second time: `board_size`, `rack_size`, `center`, `bingo_bonus`, `min_bag_for_exchange`, `premiums` (15 strings using `T`, `D`, `t`, `d` and `.`), `tile_values` and `levels`.

## Games

A game is saved after every turn. The person moves first, and each move's response already includes the bot's reply. See `docs/decisions/0006-game-sessions.md`.

| Endpoint | Body | Purpose |
|---|---|---|
| `POST /v1/games` | `{"level": "hard"}`, with optional `seed` | Start a game. Returns 201 |
| `GET /v1/games/{id}` | | Get the current state |
| `POST /v1/games/{id}/play` | `{"tiles": [{"row": 7, "col": 7, "letter": "A"}]}` | Play tiles |
| `POST /v1/games/{id}/exchange` | `{"tiles": "QU"}` | Swap tiles |
| `POST /v1/games/{id}/pass` | `{}` | Pass |
| `GET /v1/games/{id}/gcg` | | Download a finished game's record as a text file |

Each of these except the record returns the game as the person may see it:

```json
{
  "id": "6141b0d73d5c1cac9ed2dc79d8c89787",
  "level": "hard",
  "status": "active",
  "board": ["..."],
  "rack": "AEILLMT",
  "scores": { "you": 12, "bot": 44 },
  "bag_remaining": 72,
  "bot_tiles": 7,
  "can_exchange": true,
  "turns": [
    { "player": "you", "kind": "play", "position": "8H", "word": "MALE", "tiles": [], "score": 12, "total": 12 },
    { "player": "bot", "kind": "exchange", "exchanged_count": 3, "score": 0, "total": 0 }
  ]
}
```

The bot's rack, the bag and the seed are never included while the game is active, and the tiles a bot exchanged are never shown. When the game ends, `status` is `finished` and `result` is added:

```json
{
  "outcome": "loss",
  "reason": "went_out",
  "your_rack": "DOU",
  "bot_rack": "",
  "adjustments": { "you": -5, "bot": 5 }
}
```

`outcome` is `win`, `loss` or `draw`, and `reason` is `went_out` or `scoreless`.

| Status | `code` | Meaning |
|---|---|---|
| 400 | | Malformed request |
| 404 | `not_found` | No such game |
| 409 | `game_over` | The game has already finished |
| 409 | `not_finished` | A record was requested for an active game |
| 409 | `conflict` | The game changed during this request, so reload it |
| 422 | A play code above, or `cannot_exchange` | The move broke the rules, and the game hasn't changed |
