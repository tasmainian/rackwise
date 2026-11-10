// Adapted from ably-labs/word-game, src/component/tile/LetterTile.js.
// Copyright 2022 Ably Real-time Ltd. Licensed under Apache-2.0; see
// third_party/ably-word-game/LICENSE.
// Changed for Rackwise: rewritten in TypeScript as a <div> usable both on the board
// and on the rack, with blank and "last move" states and a new visual style.

export const DRAG_TYPE = "text/rackwise-tile";

interface TileProps {
  letter: string;
  value: number;
  blank?: boolean;
  tone?: "played" | "pending" | "last" | "rack";
  /** When set, the tile can be dragged, carrying this rack tile id. */
  dragId?: number;
}

export function Tile({ letter, value, blank = false, tone = "played", dragId }: TileProps) {
  return (
    <div
      className={`tile tile-${tone}${blank ? " tile-blank" : ""}`}
      draggable={dragId !== undefined}
      onDragStart={
        dragId === undefined ? undefined : (event) => event.dataTransfer.setData(DRAG_TYPE, String(dragId))
      }
    >
      <span className="tile-letter">{letter}</span>
      {value > 0 && <span className="tile-value">{value}</span>}
    </div>
  );
}
