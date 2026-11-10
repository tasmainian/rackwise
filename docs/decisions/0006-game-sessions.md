# 0006: The engine owns games and saves them after every turn

## Context

Until Phase 3, the engine was stateless. A playable game needs somewhere to keep the board, the racks and the bag, and the player must not be able to see the bot's rack or the bag order.

## Decision

The engine runs games through `internal/session`, keeping them in a Postgres `games` table. The whole state is saved as JSON after every turn. A person's move and the bot's reply are one request and one save.

- **The browser only gets a `View`.** It is built field by field from the stored game and has no field for the bot's rack, the bag or the seed until the game is over. Leaving them out of the type is safer than filtering a full record, and a test checks responses throughout a game.
- **Saves check a version number.** Each save requires the version that was loaded, so overlapping requests, such as a second tab, can't both apply to one turn. The loser gets a 409 and the page reloads the real state.
- **Finished games are stored as GCG**, with each player's rack on every turn, for the coach in Phase 4. Scores and status are also columns, for the stats page in Phase 6.
- **The engine applies its schema at startup** with `CREATE TABLE IF NOT EXISTS`. Postgres only runs `db/init` on a brand-new volume, so that folder can't add tables to an existing database.

## Why not the other options

- **Keeping games only in memory** loses them on any restart, and wouldn't work on a host that scales to zero, such as Cloud Run in Phase 8.
- **Keeping state in the browser** would hand the player the bot's rack and the bag.
- **Putting storage in the Python service** would split the rules from the game they apply to, and add a network call to every move.

## Trade-offs

- This adds the engine's first dependency, pgx, which is MIT licensed.
- With no `DATABASE_URL`, the engine falls back to an in-memory store for local development. Games are then lost on restart, and it logs a warning saying so.
- There are no accounts, so anyone who has a game's id can play it. That is fine locally, but needs revisiting before a public deployment.
- The person always moves first in this version, which is a small advantage. Win rates by bot level in Phase 6 should be read with that in mind.
- A move sent twice in a row, the second after the first finished, counts as two moves. The Submit button is disabled while a move is in flight, which covers double clicks, so request de-duplication wasn't added.
