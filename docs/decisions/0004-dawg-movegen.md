# 0004: Move generation with Appel-Jacobson on a DAWG

## Context

The coach needs every legal play for a position. The two standard algorithms are Appel and Jacobson (1988), using a DAWG, and Gordon (1994), using a GADDAG. Gordon reports the GADDAG as about twice as fast and about five times larger.

## Decision

Use Appel-Jacobson on a DAWG, built at startup with Daciuk's incremental algorithm.

## Why

- A request only needs one position. Measured on this machine, generation takes about 6 ms for a mid-game position and about 214 ms for the worst case tried, a double-blank rack on a busy board.
- It is simpler to explain. Plays are built from their first letter, and the anchor rule guarantees each is found once.
- Memory stays small. ENABLE's 168,551 playable words fit in 52,928 graph nodes, with children stored in one flat array and found by a bitmask rather than a 26-entry table per node.

## How it's tested

An independent brute-force generator lays every sequence of rack tiles from every free square in both directions, and asks `game.Validate` about each. It uses no anchors, cross-checks or graph. The two must find exactly the same plays on random positions, so the test catches missed plays as well as illegal ones. Both share one scoring routine, which has its own hand-computed tests.

## Trade-offs

Self-play for leave values and tuning would run faster on a GADDAG, but the current speed is enough. The generator could also score incrementally rather than calling `game.ScorePlay` for each play. Sharing the routine is slower, but guarantees that validation and generation can't disagree about a score.
