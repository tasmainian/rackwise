// Adapted from ably-labs/word-game, src/component/Board.js and
// src/component/tile/AvailableTile.js and BonusTile.js.
// Copyright 2022 Ably Real-time Ltd. Licensed under Apache-2.0; see
// third_party/ably-word-game/LICENSE.
// Changed for Rackwise: rewritten in TypeScript, using Rackwise's layout from the
// engine, with click-to-place as well as dragging, keyboard access and coordinates.

import { useState } from "react";

import type { Rules } from "../api";
import { type PendingTile, pendingAt } from "../placement";
import { DRAG_TYPE, Tile } from "./Tile";

const PREMIUMS: Record<string, { label: string; name: string; className: string }> = {
  T: { label: "3W", name: "triple word", className: "square-tw" },
  D: { label: "2W", name: "double word", className: "square-dw" },
  t: { label: "3L", name: "triple letter", className: "square-tl" },
  d: { label: "2L", name: "double letter", className: "square-dl" },
};

const columnName = (col: number) => String.fromCharCode(65 + col);

interface BoardProps {
  rules: Rules;
  board: string[];
  pending: PendingTile[];
  /** Squares of the opponent's last play, as "row,col". */
  highlighted: Set<string>;
  disabled: boolean;
  onSquareClick: (row: number, col: number) => void;
  onTileDrop: (tileId: number, row: number, col: number) => void;
}

export function Board({ rules, board, pending, highlighted, disabled, onSquareClick, onTileDrop }: BoardProps) {
  const indexes = [...Array(rules.board_size).keys()];
  return (
    <table className="board">
      <thead>
        <tr>
          <td />
          {indexes.map((col) => (
            <th key={col} scope="col">
              {columnName(col)}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {indexes.map((row) => (
          <tr key={row}>
            <th scope="row">{row + 1}</th>
            {indexes.map((col) => (
              <Square
                key={col}
                row={row}
                col={col}
                rules={rules}
                played={board[row][col]}
                pending={pendingAt(pending, row, col)}
                highlighted={highlighted.has(`${row},${col}`)}
                disabled={disabled}
                onClick={onSquareClick}
                onTileDrop={onTileDrop}
              />
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

interface SquareProps {
  row: number;
  col: number;
  rules: Rules;
  played: string;
  pending?: PendingTile;
  highlighted: boolean;
  disabled: boolean;
  onClick: (row: number, col: number) => void;
  onTileDrop: (tileId: number, row: number, col: number) => void;
}

function Square({ row, col, rules, played, pending, highlighted, disabled, onClick, onTileDrop }: SquareProps) {
  const [draggedOver, setDraggedOver] = useState(false);
  const premium = PREMIUMS[rules.premiums[row][col]];
  const isCenter = row === rules.center && col === rules.center;
  const name = `${columnName(col)}${row + 1}`;

  if (played !== ".") {
    const letter = played.toUpperCase();
    const blank = played !== letter;
    return (
      <td className="square">
        <div role="img" aria-label={`${name}: ${letter}${blank ? ", blank" : ""}`} className="square-content">
          <Tile letter={letter} value={blank ? 0 : rules.tile_values[letter]} blank={blank} tone={highlighted ? "last" : "played"} />
        </div>
      </td>
    );
  }

  const description = pending
    ? `${name}: ${pending.letter} placed. Click to take it back`
    : `${name}, empty${premium ? `, ${premium.name}` : ""}${isCenter ? ", start square" : ""}`;

  return (
    <td
      className={`square ${premium?.className ?? ""}${draggedOver ? " square-drag" : ""}`}
      onDragOver={(event) => event.preventDefault()}
      onDragEnter={(event) => {
        event.preventDefault();
        setDraggedOver(true);
      }}
      onDragLeave={() => setDraggedOver(false)}
      onDrop={(event) => {
        event.preventDefault();
        setDraggedOver(false);
        const tileId = event.dataTransfer.getData(DRAG_TYPE);
        if (tileId !== "") onTileDrop(Number(tileId), row, col);
      }}
    >
      <button type="button" className="square-content" aria-label={description} disabled={disabled} onClick={() => onClick(row, col)}>
        {pending ? (
          <Tile
            letter={pending.letter}
            value={pending.blank ? 0 : rules.tile_values[pending.letter]}
            blank={pending.blank}
            tone="pending"
            dragId={disabled ? undefined : pending.tileId}
          />
        ) : isCenter ? (
          <span className="square-star" aria-hidden="true">
            ✦
          </span>
        ) : premium ? (
          <span className="square-label" aria-hidden="true">
            {premium.label}
          </span>
        ) : null}
      </button>
    </td>
  );
}
