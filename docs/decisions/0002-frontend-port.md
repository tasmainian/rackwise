# 0002: New frontend that ports a few components from ably-labs/word-game

## Context

The stack calls for React and TypeScript. Of the web games reviewed in November 2026, only one is both React and permissively licensed:

- **ably-labs/word-game** (Apache-2.0): React 17, JavaScript, Create React App and MUI, last committed in March 2023. The realtime Ably client is created in `App.js`, and lobbies, login and chat are built around it.
- **woogles-io/liwords** (AGPL-3.0): the most polished, but an 834 MB multi-service repo. AGPL also covers hosted versions.
- **edlerd/scrabble** (Apache-2.0) and **amnond/jscrab** (BSD-3-Clause): no React.

## Decision

Start a new Vite + React + TypeScript app. In Phase 3, port only ably-labs' board, tile and dialog components, rewriting them in TypeScript.

The ported components take Rackwise's own board layout and visual style (see `0003-original-rules.md`). ably-labs' board layout files are not ported. In the end, three things were adapted: the board table with its drag handling, the tile, and the blank-letter dialog. The exchange and confirm dialogs were written fresh on the browser's `<dialog>`, without MUI. Click-to-place was added next to dragging, for keyboards and touch screens.

## Why

- A single-player game against a bot needs no lobbies, accounts, chat or hosted realtime service, so most of that app would be deleted anyway.
- Create React App is no longer a good base for a new project, and starting fresh avoids a migration.

## Trade-offs

- The reuse is small, and saves only a modest amount of time.
- The Apache-2.0 licence requires keeping its copyright and licence notices and marking changes, which adds a little bookkeeping. This is tracked in `THIRD_PARTY.md`.
