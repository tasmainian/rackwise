// Tiles the player is arranging before submitting. This only tracks the
// arrangement: whether it is legal, and what it scores, is for the engine to say.

import type { TilePlacement } from "./api";

export const BLANK = "?";

/** A tile on the rack. The id tells apart two tiles with the same letter. */
export interface RackTile {
  id: number;
  tile: string;
}

/** A tile placed on the board but not yet submitted. */
export interface PendingTile {
  tileId: number;
  row: number;
  col: number;
  /** The letter it shows. For a blank, this is the chosen letter. */
  letter: string;
  blank: boolean;
}

export function toRackTiles(rack: string): RackTile[] {
  return [...rack].map((tile, id) => ({ id, tile }));
}

export function pendingAt(pending: PendingTile[], row: number, col: number): PendingTile | undefined {
  return pending.find((tile) => tile.row === row && tile.col === col);
}

export function isSquareFree(board: string[], pending: PendingTile[], row: number, col: number): boolean {
  return board[row][col] === "." && !pendingAt(pending, row, col);
}

/** Places a tile, moving it if it's already on the board. */
export function placeTile(pending: PendingTile[], tile: PendingTile): PendingTile[] {
  return [...pending.filter((other) => other.tileId !== tile.tileId), tile];
}

export function removeTile(pending: PendingTile[], tileId: number): PendingTile[] {
  return pending.filter((tile) => tile.tileId !== tileId);
}

export function tilesOnRack(rack: RackTile[], pending: PendingTile[]): RackTile[] {
  return rack.filter((tile) => !pending.some((placed) => placed.tileId === tile.id));
}

/** A played blank is sent as a lowercase letter. */
export function toPlacements(pending: PendingTile[]): TilePlacement[] {
  return pending.map(({ row, col, letter, blank }) => ({
    row,
    col,
    letter: blank ? letter.toLowerCase() : letter,
  }));
}

export function shuffled<T>(items: T[], random: () => number = Math.random): T[] {
  const result = [...items];
  for (let i = result.length - 1; i > 0; i--) {
    const j = Math.floor(random() * (i + 1));
    [result[i], result[j]] = [result[j], result[i]];
  }
  return result;
}
