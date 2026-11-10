# 0003: Rackwise has its own board and tile set

## Context

Rackwise is a crossword tile game, a genre with many members. Its mechanics are common to the genre: racks of tiles, a grid, premium squares, cross-words, blanks and a bonus for using all your tiles. The classic game's exact premium layout, tile values and tile counts are what make a game a copy of that particular game, rather than a member of the genre.

## Decision

Rackwise uses an original premium layout and tile set, defined in one place: `engine/internal/game/rules.go`.

- **Board:** 15x15 with 8 triple-word squares on the edges, away from the corners. The start square has no premium.
- **Tiles:** 100, with values and counts of our own, including 5 S tiles.
- **Bingo bonus:** 40 points for playing 7 tiles.
- **End of game:** each player loses their remaining tile values, and a player who goes out gains the opponent's. A run of 6 scoreless turns also ends the game.
- **No challenges:** an invalid word is simply rejected.

## Trade-offs

- Published strategy advice and numbers, such as specific leave values, don't transfer directly. The ideas do, like balance and defence, but the numbers must come from our own self-play. This is why `cmd/genleaves` exists.
- Other engines can't check our scores, so scoring relies on hand-computed test positions.
- If rules change, leave values must be regenerated with `cmd/genleaves`.
