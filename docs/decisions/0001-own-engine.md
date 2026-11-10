# 0001: Write our own Go engine instead of adapting Macondo

## Context

The project needs a Go engine that gives legal moves, scores, leaves and three bot levels. The candidates reviewed in November 2026 were:

- **Macondo** (Go, GPL-3.0): very strong, with 697 files and over 100 dependencies. Its `data/` folder has no word lists or leave values.
- **MAGPIE** (C, GPL-3.0) and **Quackle** (C++, GPL-3.0): strong, but outside the stack and copyleft.
- **wolges** (Rust, MIT): permissive, but outside the stack.
- **GoSkrafl** (Go, CC BY-NC 4.0): bars commercial use.

## Decision

Write the engine in Go from the published algorithms: Appel and Jacobson (1988) and Gordon (1994).

## Why

- The specification only needs move generation, scoring, leaves and simple bots. That is a small, well-documented problem.
- The engine is the source of truth for the coach, so being able to explain every line of it matters in an interview.
- Adapting Macondo would put the engine service under GPL-3.0. Keeping our own code keeps the whole repo under one permissive licence.
- Macondo would still need an ENABLE lexicon and leave values built for it, so it would not remove that work.

## Trade-offs

- **"Best move" comes from a static evaluation (score plus leave value), not simulation.** The coach will sometimes call a move best when a simulating engine would prefer another. The README must state this.
- The hard bot will be weaker than one built on Macondo.
- We own the bugs. Scoring and move generation need thorough tests against known positions.

## If this changes

The engine sits behind an HTTP API, so a stronger analysis engine could later run as a separate service without rewriting the coach. That would need a fresh licence review.
