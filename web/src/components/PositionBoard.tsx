import { type KeyboardEvent } from "react";

const BOARD_SIZE = 15;
const CENTER = 7;
const COLUMNS = "ABCDEFGHIJKLMNO";

export type EntryMode = "board" | "play";

export function emptyBoard(): string[] {
  return Array<string>(BOARD_SIZE).fill(".".repeat(BOARD_SIZE));
}

function cellId(row: number, col: number): string {
  return `square-${row}-${col}`;
}

function withLetter(board: string[], row: number, col: number, letter: string): string[] {
  return board.map((line, r) => (r === row ? line.slice(0, col) + letter + line.slice(col + 1) : line));
}

const PREMIUMS: Record<string, { label: string; className: string }> = {
  T: { label: "3W", className: "square-tw" },
  D: { label: "2W", className: "square-dw" },
  t: { label: "3L", className: "square-tl" },
  d: { label: "2L", className: "square-dl" },
};

// `board` holds tiles already in the game, and `play` holds this turn's tiles. Typing changes
// whichever one `mode` selects, so a player can't overwrite the other kind by accident.
export function PositionBoard({
  board,
  play,
  premiums,
  mode,
  down,
  onChange,
}: {
  board: string[];
  play: string[];
  premiums: string[];
  mode: EntryMode;
  down: boolean;
  onChange: (grid: string[]) => void;
}) {
  const active = mode === "play" ? play : board;
  const other = mode === "play" ? board : play;

  const focus = (row: number, col: number) => {
    document.getElementById(cellId(row, col))?.focus();
  };
  const next = (row: number, col: number): [number, number] => (down ? [row + 1, col] : [row, col + 1]);

  const type = (row: number, col: number, typed: string) => {
    const letter = typed.slice(-1).toUpperCase();
    if (letter !== "" && !/^[A-Z]$/.test(letter)) return;
    if (other[row][col] !== ".") {
      // Typing over the other kind of tile moves past it, as when playing through a letter.
      if (letter) focus(...next(row, col));
      return;
    }
    onChange(withLetter(active, row, col, letter || "."));
    if (letter) focus(...next(row, col));
  };

  const keyDown = (event: KeyboardEvent, row: number, col: number) => {
    const moves: Record<string, [number, number]> = {
      ArrowUp: [row - 1, col],
      ArrowDown: [row + 1, col],
      ArrowLeft: [row, col - 1],
      ArrowRight: [row, col + 1],
    };
    const target = moves[event.key];
    if (target) {
      event.preventDefault();
      focus(target[0], target[1]);
    } else if (event.key === "Backspace" && active[row][col] === ".") {
      event.preventDefault();
      const [backRow, backCol] = down ? [row - 1, col] : [row, col - 1];
      if (backRow >= 0 && backCol >= 0) {
        if (other[backRow][backCol] === ".") onChange(withLetter(active, backRow, backCol, "."));
        focus(backRow, backCol);
      }
    }
  };

  return (
    <div className="position-board" role="group" aria-label="Your game's board">
      {board.map((line, row) =>
        [...line].map((boardCell, col) => {
          const playCell = play[row][col];
          const cell = playCell !== "." ? playCell : boardCell;
          const premium = PREMIUMS[premiums[row]?.[col] ?? ""];
          const isCenter = row === CENTER && col === CENTER;
          const filled = playCell !== "." ? "position-cell-play" : boardCell !== "." ? "position-cell-filled" : "";
          return (
            <input
              key={`${row}-${col}`}
              id={cellId(row, col)}
              className={`position-cell ${filled || (isCenter ? "position-cell-center" : (premium?.className ?? ""))}`}
              aria-label={`Square ${row + 1}${COLUMNS[col]}${premium ? ` (${premium.label})` : ""}${isCenter ? " (centre)" : ""}`}
              placeholder={premium?.label}
              autoCapitalize="characters"
              autoComplete="off"
              value={cell === "." ? "" : cell}
              onFocus={(event) => event.target.select()}
              onChange={(event) => type(row, col, event.target.value)}
              onKeyDown={(event) => keyDown(event, row, col)}
            />
          );
        }),
      )}
    </div>
  );
}
