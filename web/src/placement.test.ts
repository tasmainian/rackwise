import { describe, expect, it } from "vitest";

import {
  isSquareFree,
  type PendingTile,
  pendingAt,
  placeTile,
  removeTile,
  shuffled,
  tilesOnRack,
  toPlacements,
  toRackTiles,
} from "./placement";

const tile = (tileId: number, row: number, col: number, letter = "A", blank = false): PendingTile => ({
  tileId,
  row,
  col,
  letter,
  blank,
});

describe("placement", () => {
  it("gives identical letters different ids", () => {
    expect(toRackTiles("AA?")).toEqual([
      { id: 0, tile: "A" },
      { id: 1, tile: "A" },
      { id: 2, tile: "?" },
    ]);
  });

  it("moves a tile instead of duplicating it", () => {
    const moved = placeTile([tile(0, 1, 1)], tile(0, 2, 2));
    expect(moved).toEqual([tile(0, 2, 2)]);
  });

  it("removes only the given tile", () => {
    expect(removeTile([tile(0, 1, 1), tile(1, 1, 2)], 0)).toEqual([tile(1, 1, 2)]);
  });

  it("keeps placed tiles off the rack, even with the same letter", () => {
    const rack = toRackTiles("AA");
    expect(tilesOnRack(rack, [tile(0, 1, 1)])).toEqual([{ id: 1, tile: "A" }]);
  });

  it("finds whether a square is free", () => {
    const board = ["A.", ".."];
    const pending = [tile(0, 1, 1)];
    expect(isSquareFree(board, pending, 0, 0)).toBe(false);
    expect(isSquareFree(board, pending, 1, 1)).toBe(false);
    expect(isSquareFree(board, pending, 0, 1)).toBe(true);
    expect(pendingAt(pending, 1, 1)?.tileId).toBe(0);
  });

  it("sends a blank as a lowercase letter", () => {
    expect(toPlacements([tile(0, 7, 7, "Q"), tile(1, 7, 8, "I", true)])).toEqual([
      { row: 7, col: 7, letter: "Q" },
      { row: 7, col: 8, letter: "i" },
    ]);
  });

  it("shuffles without losing tiles or changing the input", () => {
    const items = [1, 2, 3, 4];
    const result = shuffled(items, () => 0);
    expect(items).toEqual([1, 2, 3, 4]);
    expect(result).not.toEqual(items);
    expect([...result].sort()).toEqual(items);
  });
});
