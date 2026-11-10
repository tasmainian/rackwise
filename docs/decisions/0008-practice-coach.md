# 0008: The coach tracks a game the player enters, and reviews it at the end

## Context

The developer's main goal is the coach, specifically to get better at Crossplay. Rackwise's own board and tile values make its games poor practice for that. An exact copy of Crossplay was ruled out, so the coach works on positions the player enters from games they're reviewing. The first version took a loose list of board letters, which couldn't show whether a word fits, so it was replaced with a board grid. The developer also asked to remove the bots from the app, to place their actual play on the board, and to get coaching as one summary when the game is over.

## Decision

During the game, the player copies tiles onto a 15x15 grid, enters their rack and places their play. Recording a turn checks only the geometry and shows every word formed, comma-separated, with its score. It makes no requests. A swap or pass is recorded too, and the last turn can be undone.

When the game is over, the review runs in four steps, and only the last one uses an LLM:

1. **Check the play:** word.tips is asked about every word the play formed. For this feature, word.tips replaces the engine as the source of valid words. This is the developer's deliberate change to the original ground rule. See `0007-word-additions.md` for the site's risks.
2. **Find alternatives:** `word_lookup` asks word.tips for words from the rack and each distinct set of letters in a row or column, and `board.py` places them on real squares and checks their cross-words.
3. **Compare:** `analysis.compare` works out the points behind the best play, the leave and facts about the leave for each turn, and `game_stats` adds up the game.
4. **Coach:** Claude receives the whole game and the versioned prompt, `prompts/coach/game-v2.md`, and returns one structured summary with themes and notes on the turns with real lessons.

`check_coaching` then enforces the principle that the LLM only explains. Each recommended word must be listed for that turn, and every word in capitals, position and number must appear in the data. Otherwise the whole answer is withheld. The turn review is always shown, so the screen still works with no API key or during an outage.

## Why

- **Checking output beats asking nicely.** A prompt can only tell the model not to invent a score. A check means an invented one can't reach the player.
- **One call per game, no agent.** The input is fully known, so a single structured request is cheaper and easier to test than a tool-using agent. A whole-game view can also point out habits, which per-turn coaching can't.
- **Requests wait for the end of the game.** Recording stays instant and offline, and abandoned games cost nothing.
- **Games are kept in the browser for now.** It's the smallest thing that keeps the plays. Mistake memory in Phase 6 needs them in Postgres.
- **Sonnet 5.5 at medium effort, configurable by `COACH_MODEL`.** The developer chose it to keep costs low, at half the price of Opus 5.5. It passed 16 of 17 eval cases, and one eval run of 17 calls cost $0.122.
- **The Anthropic SDK is pinned below 1.0**, because later versions need `httpx2` and the project uses `httpx`. Moving up should be a small change.

## Trade-offs

- **Many requests at the end of a game.** Each turn is capped at 12 lookups, 8 of them for lines, at one a second. A 15-turn game can make about 180 requests and take about 3 minutes. The site's `robots.txt` disallows this path, so a lower-traffic design would be better if one becomes possible.
- **Missed plays.** The site shows 30 words per word length, ranked by points, and the cap can skip lines. A better play may exist, and the result says when the search was cut off.
- **The board copies the player's multiplier layout.** This is a coaching tool for their own positions, not a playable game, so `board.PREMIUMS` uses the layout from their screenshot, with Rackwise's own colours and no logo. Check it again before publishing the repo.
- **Scores are calculated here, not by word.tips.** The site gives word points without multipliers, so the review solved its tile values from its responses (`board.TILE_VALUES`). 614 sampled words gave one exact solution, and six letters matched the player's screenshot. If the site's values change, recorded-response tests won't notice, because they use recorded data.
- **Two scoring rules aren't official.** The 40-point bonus comes from third-party guides, and multipliers counting only for newly placed tiles is an assumption from the genre.
- **The search ranks by word points, so high-multiplier plays can be missed.** The site returns its top 30 words per length without knowing the board, so a lower-value word on a 3W square may never be seen. The coach still can't judge defence, and the prompt forbids guessing.
- **Blanks already on the board count as normal letters**, because the grid has no way to mark them, so those scores can be too high. A letter that isn't on the rack is assumed to come from a blank on the rack.
- **Leave quality is qualitative.** `leaves.json` was learned for Rackwise's tile values, so its numbers aren't used here.
- **The check is crude.** It can't catch a wrong claim made in ordinary words, and it can withhold a good answer. In the `game-v2` eval run, it withheld 1 answer in 17, because Claude wrote the word endings "ED" and "ING" in capitals.
- **Evals are one-turn games with hand-written word lists and stand-in placements**, so runs are repeatable and need no lookups, but they don't test whole-game themes or fitting. `coach/tests/test_board.py` covers that. The grader matches text and can't judge advice quality.
- **The rest of the original Phase 4 is dropped from the app for now:** comparing with the engine's top moves in Rackwise games, and a post-game review screen.
- **The bots are out of the app but still on disk.** `web/src/GameApp.tsx` and `engine/` are unused and still tested. Nothing is committed, so deleting them can't be undone yet, and that's left as the developer's decision.
