# 0010: Strategy notes, retrieved by tags first and then by embeddings

## Context

Phase 5 gives the coach a library of strategy notes to cite. The developer asked how retrieval could keep coaching sessions cheap, and chose two things: tag-then-embed retrieval, and trimming the data sent to Claude. Sending the whole library with every game would add 5,792 input tokens, measured with the token counter. A trimmed 15-turn game is about 13,000.

## Decision

- **Notes:** `notes/` holds 50 short notes written for this project. Each has an ID such as `N07`, a title, tags from a fixed list, a one-line summary and a body.
- **Tags:** plain code labels each turn's problems in `analysis.mistake_tags`, using facts the review already computes, such as `duplicates`, `missed_bingo` and `q_without_u`. The labels are ordered by how many points each problem tends to cost. A turn is tagged when the play is at least 6 points behind the best play found. A poor leave or a cheaply spent blank or S is also tagged, if another play without that problem scored within 6 points.
- **Retrieval:** for each tag in order, the coach takes the nearest note with that tag. "Nearest" is measured against a short description of that problem in that turn, such as "Kept two U's, a doubled vowel." Each turn gets at most three notes. Turns with no problem get none.
- **What Claude sees:** each note's ID, title and summary, sent once per game. Turns list their `note_ids`. Claude returns `cited_notes` per turn, and the player sees the full note beside the advice.
- **The check:** `check_coaching` withholds an answer that cites a note not sent for that turn. Note titles and summaries can't contain digits or capitalised words, and a test enforces this. So quoting a note can never trip the check on words and numbers.
- **Trimming:** `coaching.build_input` sends only the fields the prompt uses. Turns without a problem send only the best play found, and mistake turns keep five, because a play with a better leave is often fourth or fifth. The JSON is sent without spaces.
- **Effort:** `COACH_EFFORT` sets the coach's effort, and defaults to `medium`.

## Why

- **Tags first:** they are exact, cheap and easy to explain. Code already knows the leave kept a duplicate U, so there's no reason to hope an embedding notices. Embeddings only choose among the few notes with the right tag, which is a ranking job a small model does well.
- **One note per tag:** in the first version, the broad `low_score` tag crowded out specific notes. A Q-without-U turn got "Weigh score and leave" before the Q note.
- **Problem-specific queries:** these make the embeddings matter. A doubled vowel ranks the doubled-vowels note first, while a doubled S ranks the note on two S's.
- **Summaries and IDs, not bodies:** this sends about a third of the tokens. Citations by ID can also be checked exactly.
- **A static embedding model:** `model2vec` with `minishlab/potion-base-8M` (MIT) needs only numpy. `onnxruntime`, which `fastembed` needs, has no wheels for Intel Macs, and torch would add hundreds of megabytes. The model loads in about 5 seconds and embeds a query in milliseconds.
- **pg8000 over psycopg:** pg8000 is BSD-licensed. psycopg is LGPL, which would have needed the developer's approval.
- **The coach creates the table, not `db/init`:** init scripts only run when the database volume is first created, and the developer's volume already existed. The `vector` column has no fixed dimension, so changing `EMBEDDING_MODEL` needs no migration. Each row stores a hash of the note and the model name, so only changed notes are embedded again. With 50 rows, no index is needed.
- **Notes are optional:** if there's no database or the model can't load, the game is coached without notes and a warning is logged.

## Measured

Input tokens from the free token counter, before any paid run:

| Input | game-v2 | game-v3 |
|---|---|---|
| System prompt | 880 | 1,123 |
| A 15-turn game shaped like real review output, with 10 mistake turns and 4 notes sent | 18,908 | 13,220 |
| The 17 eval cases together | 30,813 | 32,934 |

On a full game, trimming saves more than the notes and the longer prompt add, so input drops 30%. The eval cases are single turns with three short options, so there's little to trim and the 243-token longer prompt dominates. They go up 7%. The 15-turn game is synthetic, because no real game has been recorded yet.

After that table was measured, two more cuts were made to get a game under 5 cents: good turns went from three options to one, and the JSON lost its spaces. The 15-turn game then counts 11,044 tokens, a 42% drop from game-v2. The counter leaves out about 540 tokens per call for the output schema, found by comparing it with a billed eval run. With those added, input costs about 2.3 cents a game on Sonnet 5.5 at $2 per million input tokens. Eval cases averaged about 230 output tokens at medium effort, but a whole game's output hasn't been measured.

## Trade-offs

- **The thresholds are judgement calls.** Six points, and what counts as vowel- or consonant-heavy, were chosen by reading the eval cases' results, not tuned on real games.
- **Good turns show the coach one alternative.** That's enough to confirm a choice, but the coach can't point out a close play with a better leave on a turn that code didn't flag. The eval cases have only three options each, so they can't catch a loss from this.
- **The retrieval checks were written after seeing the results.** Each eval case's `retrieves` list names the notes that fit it, which includes what retrieval returned. These checks catch regressions, but they don't prove retrieval is good.
- **A small static model is weaker than a transformer.** With tags doing the filtering it ranks well on the eval cases, but it can still miss a better note within a tag.
- **One metadata request to huggingface.co per process**, when the model first loads. No game data is sent.
- **Notes are general advice for the genre.** They avoid specific numbers and words, so they can't contradict the data, but they can't be as specific as the turn notes either.
