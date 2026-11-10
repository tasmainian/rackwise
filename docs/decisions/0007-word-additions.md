# 0007: QI is added to the dictionary, and word.tips is a separate practice lookup

## Context

ENABLE doesn't contain QI, so the engine rejected it. The developer wants QI to be valid, and also wants to look up eligible words and their points to get better at the NYT game Crossplay.

## Decision

**In the game:** `engine/data/additions.txt` holds words accepted beyond ENABLE, currently only QI. It is merged at load time by `data.Dictionary()`, so `enable1.txt` stays byte-identical to its source. The engine is still the only authority on what is legal in Rackwise and what a play scores there.

**For practice:** the `lookup_words` MCP tool in `coach/` asks word.tips for NWL23 words that can be made from a rack and board letters. For this tool, word.tips is the only source of points, and they are returned exactly as received. The tool isn't connected to the engine or the bots.

## Why they are separate

- Move generation needs a complete local word list. It can't call a website for each candidate.
- Rackwise deliberately has its own tile values (`0003-original-rules.md`). In one sampled response, word.tips' points work out as H=3 and L=2, while Rackwise has H=4 and L=1, so the two sets of points can't be mixed.
- The project's rule against official dictionaries still applies to everything shipped in the game.

## Trade-offs and risks

- **Site rules:** `robots.txt` on word.tips disallows `/unscramble/*/`, which includes this endpoint. Its terms don't mention automated access. The developer chose to use it after being told. The tool makes one request per call, at most one a second, stores nothing and never fetches later result pages. Position reviews share the same rules, with up to 20 requests per review (`0008-practice-coach.md`).
- **Undocumented endpoint:** it's the site's internal page-data format and can change without notice. The tool checks the response's shape and fails with a clear error.
- **Not Crossplay's list:** third-party sites describe Crossplay's dictionary as an unpublished subset of NWL23, so a listed word might still be rejected there.
- **Points are for the word only:** they don't include premium squares, cross-words or bonuses.
- **Partial results:** the site shows 30 words per word length. `more_available` says when some were cut off.
- **Licensing:** NWL23 is licensed by NASPA. The repo holds only two small recorded responses as test fixtures, and no list is stored. Decide whether to keep this tool before publishing the repo.
- **Leave values:** `leaves.json` was learned without QI, and relearning it hasn't been done. The effect should be small, but it hasn't been measured.
