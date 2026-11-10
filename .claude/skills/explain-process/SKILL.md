---
name: explain-process
description: Helps the Rackwise developer explain and rehearse how the project was built, for job interviews. Use when they ask to explain their development process, prepare for an interview, practise questions about Rackwise, or defend a design decision.
---

# Explain the Rackwise development process

The person using this skill built Rackwise by directing Claude Code. Help them explain that process truthfully and in their own voice.

## Rules

1. **Be exact about who did what.** Use the "Who did what" section. Never say the developer personally wrote, debugged or reviewed something unless they tell you they did.
2. **Don't hide the AI's role.** If the story only works when Claude Code is left out, it's the wrong story. The strongest version shows direction, constraints and checking of results.
3. **Check the current state first.** This file is a record up to the date below. Read `CLAUDE.md`, `docs/decisions/` and `git log`, and trust them where they differ from this file.
4. **Only describe what exists.** The coach is the project's main feature. In this record it passes 16 of 17 eval cases but hasn't been judged on a real game, so check `evals/results/` and ask the developer before describing how well it works.
5. **Keep measurements as measured.** Quote the numbers below with their limits, and don't round them into stronger claims.
6. **Ask, don't invent.** Ask the developer for their motives, feelings, or anything marked "ask the developer" here. Don't fill those gaps with plausible detail.
7. **Avoid brand claims.** The product is Rackwise. Mention other word games only when explaining a decision.

## How to help

- **"Explain my process":** give the short version, then offer to go deeper on any phase.
- **"Prepare me" or "quiz me":** ask one question at a time from "Likely questions", wait for their answer, then say what was accurate, what was missing and what overclaimed. Check their answer against the code.
- **"Why did we choose X?":** read the matching decision record and answer with context, decision and trade-off.
- **Technical follow-ups:** open the code and walk them through it. They should be able to explain move generation, scoring, leave values and the game save path without notes.

## The short version

> I built Rackwise, an AI coach for crossword word games: you track your plays on a board, and get a coaching summary when the game is over. I used Claude Code as the implementer and kept the product and engineering direction myself. Before any code, I wrote a phased plan and hard constraints: the LLM can never decide a word's validity or score, every reused licence gets checked, and secrets stay in environment variables. Each phase ended with a summary and a list of things to test by hand, and the next one started only after I approved it. So far there's a React frontend and an end-of-game coach, running with one Docker Compose command. I first built a Go engine with three bots and saved games, then took the bots out of the app when I decided the coach was what mattered. The main checks are independent of the code they test, such as comparing move generation against a brute-force generator on random positions.

Update this as phases finish. Before an interview, the developer should rewrite it in their own words.

## Who did what

| The developer | Claude Code |
|---|---|
| Set the project goal, stack and eight-phase plan | Researched existing projects and licences |
| Set the ground rules below | Wrote the code, tests, configuration and decision records |
| Chose "own engine, ported UI" from the options presented | Proposed options and recommendations, including original rules and the DAWG |
| Approved the Phase 1 and Phase 2 plans, and started Phase 3 | Ran tests, lint, self-play tournaments and browser checks, and reported results |
| Ran it locally and reported failures, such as the port clash | Diagnosed and fixed those failures |
| Challenged the rejection of QI, which Crossplay accepts | Traced the cause, made the fix and raised the risks in the proposed lookup tool |

**Ask the developer:** how much of the code they've read, which parts they can explain unaided, and how much they've played by hand. Their answers decide how strongly they can claim understanding.

## The ground rules, and why they matter

- **The LLM never decides validity or scores.** The engine is the source of truth, and the LLM only explains its output. This prevents the most damaging kind of coach hallucination.
- **Licences are checked before reuse, and copyleft needs the developer's sign-off.** Everything reused is in `THIRD_PARTY.md`.
- **Only openly licensed word lists, no official dictionaries, no brand names.**
- **Tests are written as you go, using known answers.**
- **Secrets come from environment variables, and prompts are versioned files.**
- **Non-obvious decisions are written down**, so they can be defended later.
- **Each phase stops for approval**, with a summary and a hand-test list.

## How each phase went

### Phase 1: research and setup (done)

- **Compared:** Macondo, MAGPIE, Quackle, wolges, GoSkrafl, woogles-io/liwords, ably-labs/word-game, edlerd/scrabble and amnond/jscrab.
- **Decided:** write an own Go engine, and start a new Vite and React app that ports only a few ably-labs components. Macondo is GPL-3.0 and would put the engine under copyleft. An own engine is also explainable line by line. See decision records 0001 and 0002.
- **Built:** repo scaffold, Compose, CI and README skeleton.
- **Rejected on inspection:** edlerd/scrabble's word list, whose word count matches a published TWL06 copy, and `google-10000-english`, whose licence is too restrictive.
- **Hit:** Docker wasn't installed, then port 5432 clashed with a local PostgreSQL. The host port moved to 5433, and the developer's own database was left alone.

### Phase 2: engine and bots (done)

- **Original rules:** Rackwise has its own premium layout, tile values and counts, so it's a member of the genre and not a renamed copy of one game (0003). The cost is that published leave numbers don't apply and other engines can't check its scores.
- **Move generation:** Appel-Jacobson on a DAWG, simpler and smaller than a GADDAG (0004). About 6 ms mid-game and about 214 ms for a double-blank rack, on one machine.
- **Leave values:** ridge regression over tiles and pairs, learned from 40,000 self-play games, giving 405 inspectable numbers (0005).
- **Bots:** easy plays common words, using SCOWL intersected with ENABLE. Medium takes the highest score. Hard uses score plus leave value, minus a lane penalty.
- **Key test:** an independent brute-force generator must find exactly the same plays on random positions, so missed plays are caught as well as illegal ones.

Measured with `cmd/selfplay`, on seeds not used for tuning. Ranges are 95% confidence:

| Match | Games | Win rate | Mean spread |
|---|---|---|---|
| Hard vs medium | 6,000 | 60.0% +/- 1.2 | +23.2 +/- 2.3 |
| Medium vs easy | 2,000 | 99.9% +/- 0.2 | +242.9 +/- 3.9 |
| Hard vs easy | 2,000 | 99.8% +/- 0.2 | +261.9 +/- 3.8 |

**An honest result worth telling:** the specification asked for a penalty on opening triple-word lanes. It was built, and it showed no measured benefit in a direct match (49.6% +/- 1.3 for the side without it). The same was true of the exchange threshold. So the hard bot's edge comes from leave values, and the record says so.

### Phase 3: playable game (done, awaiting sign-off)

- **Built:** board, rack, click and drag placement, submit, exchange, pass, bot picker, scores, move list and GCG download with both racks on every turn.
- **Saved games:** the engine saves full game state in Postgres after every turn, with a version check so overlapping requests can't both apply (0006).
- **Hidden information:** the browser gets a `View` type with no field for the bot's rack, bag or seed. Leaving them out of the type is safer than filtering a full record.
- **One set of rules:** the frontend reads layout and values from `/v1/rules`.
- **Checked in a real browser:** a full game, including an illegal play, a blank, an exchange, a pass, a refresh and the GCG download. An engine restart kept the game, and overlapping saves returned a 409.
- **Bugs only a real browser found:** squares rendered 44x18 because `aspect-ratio` doesn't apply to table cells, and the action buttons fell below the fold. Mocked component tests passed throughout.
- **Known limits:** an automated drag to an off-screen square failed in a short scrolling window and wasn't fully root-caused, though clicking works. Phone squares are small. Only Chromium was tested. GCG was checked with our own parser, not a third-party tool. There are no accounts. CI has never run, because nothing has been pushed.

### The QI question (open when this was written)

Good example of a requirement meeting a constraint:

1. ENABLE doesn't contain QI, so the engine rejected it. This had been documented as a known limit.
2. The developer said it should be valid, because the NYT game Crossplay accepts it.
3. Claude Code added a separate `engine/data/additions.txt`, so `enable1.txt` stays byte-identical to its source, and tests were updated. QI scores 11 on the centre row.
4. The developer then asked for an MCP tool that looks up eligible words on word.tips with `dictionary=nwl23`.
5. Claude Code checked first and held off building it. The site's `robots.txt` disallows that path, NWL23 is a licensed official list that conflicts with the project's own rule, third-party sites describe Crossplay's list as an unpublished subset of NWL23, and move generation needs a whole local list, not per-word lookups.
6. Three alternatives went to the developer: a hand-curated additions file, a locally held licensed list, or asking the site for permission.
7. Knowing those risks, the developer chose the word.tips lookup, because they want to get better at Crossplay, and made the site the only source of points for it.
8. Claude Code built it as a separate MCP tool, `lookup_words`, with one request per call and nothing stored. The game keeps its own rules and scoring (decision record 0007).

The developer made this trade-off knowingly, and it goes against the site's `robots.txt`, so don't describe it as approved by the site. The bot strength and leave figures above were measured before QI was added, and the leave values haven't been relearned.

### Phase 4: coach v1 (done)

- **Direction change:** the developer asked for an exact copy of Crossplay using a reverse-engineering tool. Claude Code declined that and offered a practice companion, which the developer chose because the coach is what they care about most.
- **Built:** the player tracks their game on a grid, placing each play and entering their rack. At the end, word.tips checks the words and supplies alternatives, plain code fits and scores them with the board's multipliers, and Claude summarises the game under a versioned prompt.
- **The developer caught a design flaw:** the first version took a loose list of board letters. They asked how it could know where the letters were, and it couldn't, so it was rebuilt around a real board. They also had the bots removed from the app, and the code stays on disk until they decide whether to delete it.
- **Then a coaching change:** the developer asked to place the actual play on the board, keep a log of plays, and get coaching only as one summary when the game is over. That made recording instant and offline, and cut Claude to one call per game.
- **Key design:** `check_coaching` withholds any answer with a recommendation, game word or number outside the data, so "the LLM only explains" is enforced in code.
- **Board from a picture:** the developer asked to upload a screenshot instead of typing tiles. Claude's vision transcribes it and the player checks the grid (decision record 0009). Don't quote its accuracy until it's been measured on real pictures.
- **Evals:** 17 cases, 15 single turns and 2 whole games, with a text-matching grader.
  - `game-v1` passed 12 of 17. Reading the failures showed three grader or check problems, one bad test case and one real coaching gap.
  - `game-v2` passed 16 of 17. The one failure was the check correctly withholding "ED" and "ING" written in capitals.
  - Each run cost about $0.12.
  - Be honest that the grader and data were corrected between runs, so the jump from 12 to 16 isn't all down to the prompt.
- **Cost work:** shrinking pictures in the browser cut image tokens from 3,959 to 1,459, measured with the token counter. A real picture read cost $0.036, mostly in output.
- **Ground rule changed:** word.tips replaced the engine as the source of truth for this feature, by the developer's decision (decision record 0008).
- **Sign-off:** the developer signed Phase 4 off on the eval results, without a real game being measured.

### Phase 5: strategy notes and retrieval (built, waiting for sign-off)

- **The developer's question started it:** they asked how RAG could make coaching sessions cheaper. Claude Code answered that RAG usually adds tokens, and that its real value here was keeping Phase 5's notes from bloating the prompt. The developer chose tag-then-embed retrieval and trimming the data sent to Claude.
- **Built:** 50 strategy notes. Plain code tags each turn's problems, retrieval takes the nearest note per tag from pgvector, Claude sees only note IDs and summaries, and the check withholds any citation of a note that wasn't sent (decision record 0010).
- **Retrieval was tuned by reading its output.** In the first version a broad tag crowded out specific notes, and small-gap leave mistakes got no notes at all. Both were fixed, along with two mistagged notes.
- **A dependency problem was caught early.** `onnxruntime` has no Intel Mac wheels, so `fastembed` was swapped for `model2vec`, which needs only numpy. Installing it quietly pulled in `httpx2`, which the project avoids, so `huggingface_hub` is pinned below 2. pg8000 was chosen over psycopg because psycopg is LGPL.
- **Measured honestly:** on a realistic 15-turn game, input dropped from 18,908 to 13,220 tokens. On the small eval cases it rose 7%, because the longer prompt outweighs what trimming saves there. Both numbers are in the README.
- **Paid eval runs** at medium and low effort are waiting for the developer's approval.

### Phases 6 to 8 (planned)

- **Phase 6, mistake memory:** stored mistakes, recurring patterns and a stats page.
- **Phase 7, MCP server:** tools such as `analyze_position` and `review_game`.
- **Phase 8, ship:** deploy, finish the README, and write `docs/LEARNINGS.md`.

## Process lessons the record supports

- **Independent checks beat more tests.** The brute-force generator and the real browser run caught what same-logic and mocked tests can't.
- **Measure claims, including ones the spec assumes.** The lane penalty is the example.
- **Written constraints keep working.** The licence and dictionary rules stopped a risky shortcut in Phase 1 and again with the lookup tool.
- **Check external sources before depending on them**, for licence, provenance and site rules.
- **AI output needs supervision, and mistakes happened.** Examples from the record:
  - A careless command started a stray engine and wrote a stray file, and both were cleaned up and reported.
  - Several test fixtures were wrong at first.
  - A test game the developer seems to have played lost its saved link when the test browser was cleared. Its database row was kept, and this was reported.

## Likely questions

1. Walk me through how you used AI on this. What did you do yourself?
2. How did you know the AI's code was correct?
3. Why write an engine rather than use Macondo?
4. Why can't the LLM decide whether a word is valid?
5. How does move generation work, and why a DAWG and not a GADDAG?
6. How are leave values learned, and what are their limits?
7. Hard beats medium only 60% of the time. Is that good? What would improve it?
8. How do you keep the bot's rack hidden from the player?
9. What happens if two requests hit the same game at once?
10. Why save the whole game state as JSON rather than normalised tables?
11. How did you handle licences, and what did you reject?
12. Tell me about a time the AI's work was wrong, or a time you changed direction.
13. Why does "best move" not mean what a simulating engine means?
14. What would you change before putting this in front of real users?
15. How will you measure whether the coach is any good?
16. Why tags before embeddings? Did RAG actually save money?

## Last updated

November 2026, during Phase 5. Nothing had been committed at that point, so `git log` can't yet back up this timeline. Update this file at the end of each phase.
