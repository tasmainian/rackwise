# Strategy notes

Fifty short notes on crossword game strategy, written for this project. They're embedded into pgvector, and the coach cites them in its advice. See `docs/decisions/0010-strategy-retrieval.md`.

Each file looks like this:

```
---
id: N01
title: Duplicate letters slow your next turn
tags: duplicates
summary: One line that Claude sees.
---

The body, which only the player sees.
```

Tags must come from the list in `coach/src/rackwise_coach/notes.py`. Titles and summaries can't contain digits or capitalised words, because Claude may quote them and the coach's check would reject those. Run `python -m rackwise_coach.notes` from `coach/` to check every note. In Docker, edited notes are embedded again on the next summary, with no rebuild.
