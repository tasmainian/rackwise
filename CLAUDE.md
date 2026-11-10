# Rackwise

A coach for crossword word games: the player tracks a game they're playing elsewhere and gets an AI coaching summary when it's over. The robot opponent was built first, and has been removed from the app. **The coach is the main feature.** This is a portfolio project, so every design decision has to be defensible in an interview.

## Stack

| Part | Technology | Folder |
|---|---|---|
| Game engine and bots | Go | `engine/` |
| Coach and MCP server | Python (FastAPI) | `coach/` |
| Frontend | React + TypeScript (Vite) | `web/` |
| Storage and embeddings | Postgres with pgvector | `db/` |
| Local runtime | Docker Compose (`docker compose up`) | `docker-compose.yml` |

Other folders: `prompts/` (versioned coach prompts), `notes/` (strategy library), `evals/` (coach eval set), `docs/decisions/` (design decision records).

## Ground rules

- **The LLM never decides whether a word is valid or what it scores.** In the Rackwise game those come from the engine. In the practice coach they come from word.tips, at the user's request. The LLM only explains that data, and `check_coaching` withholds any answer that goes beyond it.
- Use only openly licensed word lists (ENABLE). Never use official tournament dictionaries, and never reuse a word list bundled in another repo without checking where it came from.
- No "Scrabble" or NYT/Crossplay branding in the UI or repo name. The name is Rackwise.
- Check the licence before reusing any code. Only use code whose licence allows modifying and publishing it, and record it in `THIRD_PARTY.md`. **Tell the user before adopting anything copyleft (GPL, AGPL).**
- Write tests as you go, especially for scoring and move generation, using known positions with known correct answers.
- Read secrets such as `ANTHROPIC_API_KEY` from environment variables and never commit them. Keep `.env.example` up to date.
- Keep prompts in versioned files under `prompts/`, never as inline strings.
- Explain non-obvious design decisions in short comments or in `docs/decisions/`.

## Working process

Work through the phases in order. After each phase, stop, summarize what was built and why, list what to test by hand, and wait for the user's approval before starting the next one. Commit or push only when the user asks.

## Phase plan

1. **Research and setup** (done): compare existing projects, choose a combination, scaffold the repo, Compose, CI and README skeleton.
2. **Engine and robot player** (done): board, bag, racks, scoring, validation and move generation. HTTP API that returns every legal move ranked by score, with the leave for each. Easy, medium and hard bots. Tests for scoring, cross-words, blanks, bingos and end-of-game penalties.
3. **Playable game** (built, then removed from the app at the user's request; the code stays in `web/src/GameApp.tsx` and `engine/` until the user decides whether to delete it): port the frontend (board, rack, placing tiles, submit, exchange, pass, bot picker, score display, move list). Save finished games as GCG, including the human's rack every turn.
4. **Coach v1** (done; signed off 2026-11-09 on the eval results, without a real game being measured): the player tracks their game on a grid, placing each play and entering their rack. When the game is over, find plays that fit using word.tips (points behind, cross-words, leave quality), send the whole game to Claude with a rubric, and show one summary. 15 eval positions and a scoring script. See `docs/decisions/0008-practice-coach.md`. Comparing moves in Rackwise games with the engine's top moves is postponed.
5. **Strategy library and RAG** (done; signed off 2026-11-10 on the free checks, with no paid eval run of game-v3 and no full game measured): 50 strategy notes embedded in pgvector with a configurable local embedding model. Retrieve up to 3 notes per mistake and cite them. See `docs/decisions/0010-strategy-retrieval.md`.
6. **Mistake memory:** store each mistake with a structured description and embedding, point out recurring patterns with links to past games, and add a stats page.
7. **MCP server:** tools such as `analyze_position`, `review_game`, `list_my_games` and `find_similar_mistakes`. Document connecting it to Claude Code and Claude Desktop.
8. **Ship it:** deploy (Google Cloud Run or simpler), finish the README, and write `docs/LEARNINGS.md`.

## Decisions so far

- **Own Go engine, not Macondo.** See `docs/decisions/0001-own-engine.md`.
- **"Best move" means score plus leave value, not simulation.** The coach will sometimes call a move best when a simulating engine would disagree. The README must say so.
- **New Vite + TypeScript frontend**, porting only ably-labs/word-game's board, tile and dialog components in Phase 3. See `docs/decisions/0002-frontend-port.md`.
- **Rackwise has its own premium layout, tile values and tile counts**, so it stays a member of the genre and not a renamed copy of one game. They live in `engine/internal/game/rules.go`. See `docs/decisions/0003-original-rules.md`.
- **Move generation is Appel-Jacobson on a DAWG**, checked against a brute-force generator. See `docs/decisions/0004-dawg-movegen.md`.
- **Leave values are learned from self-play**, so they must be regenerated with `cmd/genleaves` if the rules change. See `docs/decisions/0005-leave-values.md`.
- **The easy bot's common words come from SCOWL.** `google-10000-english` was rejected over its licence.
- **The engine owns games and saves them in Postgres after every turn.** The browser only ever gets a `View`, which has no bot rack, bag or seed. See `docs/decisions/0006-game-sessions.md`.
- **The frontend gets the layout and tile values from `/v1/rules`**, so rules are defined only in the engine.
- **ENABLE has no QI or ZA.** QI is added back through `engine/data/additions.txt` at the user's request, and ZA is still rejected. The README must say so. `leaves.json` hasn't been relearned since. See `docs/decisions/0007-word-additions.md`.
- **The review board copies the multiplier layout of the user's game, at their request.** It's a coaching tool, not a playable game: use Rackwise's own colours and no logo or brand name in the UI. Tile values were solved from word.tips' points. See `docs/decisions/0008-practice-coach.md`.
- **The board can be filled in from a picture, using Claude's vision.** It only transcribes, and the player checks the grid. See `docs/decisions/0009-board-from-picture.md`.
- **Coaching happens once, at the end of the game, at the user's request.** Recording a turn makes no word.tips or Claude requests. Games are kept in the browser until Phase 6 needs Postgres.
- **The bots are out of the app at the user's request.** The app only tracks and reviews games. The user has said not to commit or delete yet. Don't delete `web/src/GameApp.tsx`, `SetupScreen`, `GameScreen` or engine code unless the user decides to.
- **The `lookup_words` MCP tool is a separate practice aid for Crossplay**, and the user's chosen exception to the word-list rule. word.tips is its only source of points, returned unchanged. The practice coach uses the same lookup at the user's request, but it must not feed the engine or bots unless the user decides otherwise. Its use goes against that site's `robots.txt`, so keep requests low and never bulk fetch: one per `lookup_words` call, at most 12 per reviewed turn, at most one a second, and nothing stored.
- **edlerd/scrabble's bundled word list is never used.** It has the same word count as a published TWL06 copy, so it is likely an official dictionary.
- **The repo licence is MIT**, as a default the user can change.
- **The GitHub repo should be named `rackwise`.** The local folder is still called `scrabble-coach`, which only the user should rename.
- **The Anthropic SDK and MCP SDK are pinned to versions that use `httpx`** (`anthropic<1`, `mcp<2`), because later ones need `httpx2`.
- **The MCP server lives in `coach/`** so it can share the coach's code.
- **Strategy notes are retrieved by tags first, then by embeddings.** Plain code tags each turn's problems, and the embedding ranks notes within a tag. Claude sees only note IDs and summaries, and `check_coaching` rejects citations of notes that weren't sent. Note titles and summaries can't contain digits or capitalised words. See `docs/decisions/0010-strategy-retrieval.md`.
- **Embeddings use `model2vec`**, because `onnxruntime` has no Intel Mac wheels. `huggingface_hub` is pinned below 2, because 2.x needs `httpx2`. **Postgres is reached with pg8000**, not psycopg, which is LGPL.

## Conventions

- **Go:** `gofmt`, `go vet`, golangci-lint. Tests sit next to the code as `*_test.go`.
- **Python:** ruff for linting and formatting, pytest for tests. Dependencies are declared in `coach/pyproject.toml`.
- **TypeScript:** strict mode, ESLint, vitest.
- **Configuration:** services read everything from environment variables, documented in `.env.example`.
- **Decision records:** `docs/decisions/NNNN-short-title.md`, each with context, decision and trade-offs.

The web container has no source volume, so frontend changes need `docker compose up --build -d web`.

## Commands

```sh
docker compose up --build         # run everything

(cd engine && go test ./...)      # engine tests
(cd engine && TEST_DATABASE_URL=postgresql://rackwise:rackwise@localhost:5433/rackwise go test ./internal/session)   # also tests real Postgres
(cd coach && TEST_DATABASE_URL=postgresql://rackwise:rackwise@localhost:5433/rackwise pytest tests/test_notes.py)   # notes against real pgvector
(cd coach && python -m rackwise_coach.notes)   # check every note file is valid
(cd engine && go run ./cmd/selfplay -a hard -b medium -games 2000)   # bot tournament
(cd engine && go run ./cmd/genleaves -games 20000)   # relearn leave values after any rule change
(cd coach && python ../evals/score.py --check)   # grade the eval's reference answers, with no API calls
(cd coach && python ../evals/score.py)   # run the coach evals: 17 Claude calls, about $0.12; ask the user first
(cd coach && pytest && ruff check .)   # coach tests and lint (inside a venv with `pip install -e ".[dev]"`)
(cd web && npm test && npm run lint && npm run typecheck)   # frontend tests, lint and types
```
