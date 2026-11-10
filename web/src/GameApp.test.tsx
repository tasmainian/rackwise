import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { GameApp as App } from "./GameApp";

const rules = {
  board_size: 15,
  rack_size: 7,
  center: 7,
  bingo_bonus: 40,
  min_bag_for_exchange: 7,
  premiums: Array(15).fill("..............."),
  tile_values: { A: 1, C: 3, T: 1, Q: 10, E: 1, S: 1, R: 1 },
  levels: ["easy", "medium", "hard"],
};

const emptyBoard = Array(15).fill("...............");

const newGame = {
  id: "g1",
  level: "medium",
  status: "active",
  board: emptyBoard,
  rack: "ACT?ERS",
  scores: { you: 0, bot: 0 },
  bag_remaining: 86,
  bot_tiles: 7,
  can_exchange: true,
  turns: [],
};

type Route = { status?: number; body: unknown };

/** Fakes the engine, recording requests. Responses for a path are used in order. */
function fakeEngine(routes: Record<string, Route | Route[]>) {
  const calls: { path: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      const path = url.replace("/api/engine/v1", "");
      calls.push({ path, body: init?.body ? JSON.parse(init.body as string) : undefined });
      const route = routes[path];
      if (!route) return { ok: false, status: 404, json: async () => ({ error: "not found", code: "not_found" }) };
      const next = Array.isArray(route) ? route.shift()! : route;
      const status = next.status ?? 200;
      return { ok: status < 400, status, json: async () => next.body };
    }),
  );
  return calls;
}

async function startGame(routes: Record<string, Route | Route[]> = {}) {
  const calls = fakeEngine({ "/rules": { body: rules }, "/games": { status: 201, body: newGame }, ...routes });
  const user = userEvent.setup();
  render(<App />);
  await user.click(await screen.findByRole("button", { name: /medium/i }));
  await screen.findByRole("group", { name: "Your rack" });
  return { calls, user };
}

const square = (name: string) => screen.getByRole("button", { name: new RegExp(`^${name}[:,]`) });
const rackTile = (name: string) => screen.getByRole("button", { name });

describe("App", () => {
  beforeEach(() => localStorage.clear());
  afterEach(() => vi.unstubAllGlobals());

  it("offers each difficulty and starts a game at the chosen one", async () => {
    const { calls } = await startGame();
    expect(calls.find((call) => call.path === "/games")?.body).toEqual({ level: "medium" });
    expect(localStorage.getItem("rackwise.gameId")).toBe("g1");
    expect(screen.getByText("86 tiles in the bag")).toBeInTheDocument();
  });

  it("resumes a saved game after a refresh", async () => {
    localStorage.setItem("rackwise.gameId", "g1");
    fakeEngine({ "/rules": { body: rules }, "/games/g1": { body: { ...newGame, scores: { you: 12, bot: 9 } } } });
    render(<App />);
    expect(await screen.findByText("12", { selector: ".score-value" })).toBeInTheDocument();
  });

  it("goes back to choosing when a saved game no longer exists", async () => {
    localStorage.setItem("rackwise.gameId", "gone");
    fakeEngine({ "/rules": { body: rules } });
    render(<App />);
    expect(await screen.findByText("Choose your opponent")).toBeInTheDocument();
    expect(localStorage.getItem("rackwise.gameId")).toBeNull();
  });

  it("explains when the engine can't be reached", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("refused")));
    render(<App />);
    expect(await screen.findByRole("alert")).toHaveTextContent("Can't reach the game engine");
  });

  it("places tiles by clicking and submits them", async () => {
    const after = {
      ...newGame,
      board: ["C.A.T..........", ...emptyBoard.slice(1)],
      rack: "QQQ?ERS",
      scores: { you: 5, bot: 3 },
      turns: [
        { player: "you", kind: "play", word: "AT", position: "8H", score: 5, total: 5 },
        { player: "bot", kind: "pass", score: 0, total: 0 },
      ],
    };
    const { calls, user } = await startGame({ "/games/g1/play": { body: after } });

    expect(screen.getByRole("button", { name: "Submit" })).toBeDisabled();
    await user.click(rackTile("Tile A"));
    await user.click(square("H8"));
    await user.click(rackTile("Tile T"));
    await user.click(square("I8"));
    expect(screen.queryByRole("button", { name: "Tile A" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Submit" }));

    expect(calls.find((call) => call.path === "/games/g1/play")?.body).toEqual({
      tiles: [
        { row: 7, col: 7, letter: "A" },
        { row: 7, col: 8, letter: "T" },
      ],
    });
    expect(await screen.findByText("AT at 8H")).toBeInTheDocument();
    expect(screen.getByText("Passed")).toBeInTheDocument();
    expect(screen.getByText("5", { selector: ".score-value" })).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Tile Q" })).toHaveLength(3);
  });

  it("takes a placed tile back when clicked, and recalls them all", async () => {
    const { user } = await startGame();
    await user.click(rackTile("Tile A"));
    await user.click(square("H8"));
    await user.click(square("H8"));
    expect(rackTile("Tile A")).toBeInTheDocument();

    await user.click(rackTile("Tile C"));
    await user.click(square("A1"));
    await user.click(screen.getByRole("button", { name: "Recall" }));
    expect(rackTile("Tile C")).toBeInTheDocument();
  });

  it("doesn't place a tile on one already placed", async () => {
    const { user } = await startGame();
    await user.click(rackTile("Tile A"));
    await user.click(square("H8"));
    await user.click(rackTile("Tile C"));
    await user.click(square("H8"));
    expect(rackTile("Tile C")).toBeInTheDocument();
    expect(square("H8")).toHaveAccessibleName(/A placed/);
  });

  it("shows the engine's reason for an illegal play and keeps the tiles", async () => {
    const { user } = await startGame({
      "/games/g1/play": { status: 422, body: { code: "invalid_word", error: "ZA is not a valid word" } },
    });
    await user.click(rackTile("Tile A"));
    await user.click(square("H8"));
    await user.click(screen.getByRole("button", { name: "Submit" }));

    expect(await screen.findByText("ZA is not a valid word")).toBeInTheDocument();
    expect(square("H8")).toHaveAccessibleName(/A placed/);
  });

  it("asks which letter a blank stands for, and sends it lowercase", async () => {
    const { calls, user } = await startGame({ "/games/g1/play": { body: newGame } });
    await user.click(rackTile("Blank tile"));
    await user.click(square("H8"));

    const dialog = screen.getByRole("dialog", { name: /blank/i });
    await user.click(within(dialog).getByRole("button", { name: "E" }));
    expect(square("H8")).toHaveAccessibleName(/E placed/);

    await user.click(screen.getByRole("button", { name: "Submit" }));
    expect(calls.find((call) => call.path === "/games/g1/play")?.body).toEqual({
      tiles: [{ row: 7, col: 7, letter: "e" }],
    });
  });

  it("keeps a blank on the rack if choosing a letter is cancelled", async () => {
    const { user } = await startGame();
    await user.click(rackTile("Blank tile"));
    await user.click(square("H8"));
    const dialog = screen.getByRole("dialog", { name: /blank/i });
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(rackTile("Blank tile")).toBeInTheDocument();
  });

  it("exchanges the chosen tiles", async () => {
    const after = {
      ...newGame,
      turns: [
        { player: "you", kind: "exchange", exchanged: "AC", exchanged_count: 2, score: 0, total: 0 },
        { player: "bot", kind: "exchange", exchanged_count: 3, score: 0, total: 0 },
      ],
    };
    const { calls, user } = await startGame({ "/games/g1/exchange": { body: after } });
    await user.click(screen.getByRole("button", { name: "Exchange" }));

    const dialog = screen.getByRole("dialog", { name: "Exchange tiles" });
    const confirm = within(dialog).getAllByRole("button", { name: /^Exchange/ })[0];
    expect(confirm).toBeDisabled();
    await user.click(within(dialog).getByRole("button", { name: "A" }));
    await user.click(within(dialog).getByRole("button", { name: "C" }));
    await user.click(within(dialog).getByRole("button", { name: "Exchange 2" }));

    expect(calls.find((call) => call.path === "/games/g1/exchange")?.body).toEqual({ tiles: "AC" });
    expect(await screen.findByText("Exchanged AC")).toBeInTheDocument();
    expect(screen.getByText("Exchanged 3 tiles")).toBeInTheDocument();
  });

  it("doesn't offer an exchange when the bag is too small", async () => {
    localStorage.setItem("rackwise.gameId", "g1");
    fakeEngine({ "/rules": { body: rules }, "/games/g1": { body: { ...newGame, can_exchange: false, bag_remaining: 3 } } });
    render(<App />);
    expect(await screen.findByRole("button", { name: "Exchange" })).toBeDisabled();
  });

  it("passes after confirming", async () => {
    const { calls, user } = await startGame({ "/games/g1/pass": { body: newGame } });
    await user.click(screen.getByRole("button", { name: "Pass" }));
    const dialog = screen.getByRole("dialog", { name: /pass/i });
    await user.click(within(dialog).getByRole("button", { name: "Pass" }));
    expect(calls.some((call) => call.path === "/games/g1/pass")).toBe(true);
  });

  it("shows the result and record link when the game ends", async () => {
    localStorage.setItem("rackwise.gameId", "g1");
    fakeEngine({
      "/rules": { body: rules },
      "/games/g1": {
        body: {
          ...newGame,
          status: "finished",
          can_exchange: false,
          rack: "",
          scores: { you: 300, bot: 250 },
          result: { outcome: "win", reason: "went_out", your_rack: "", bot_rack: "QT", adjustments: { you: 11, bot: -11 } },
        },
      },
    });
    render(<App />);
    expect(await screen.findByText("You won!")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Download game record/ })).toHaveAttribute(
      "href",
      "/api/engine/v1/games/g1/gcg",
    );
    expect(screen.queryByRole("button", { name: "Submit" })).not.toBeInTheDocument();
    expect(screen.getByText(/bot QT/)).toBeInTheDocument();
  });

  it("shows the real state if the game moved on in another tab", async () => {
    const moved = { ...newGame, scores: { you: 44, bot: 0 } };
    const { user } = await startGame({
      "/games/g1/pass": { status: 409, body: { code: "conflict", error: "the game changed" } },
      "/games/g1": { body: moved },
    });
    await user.click(screen.getByRole("button", { name: "Pass" }));
    const dialog = screen.getByRole("dialog", { name: /pass/i });
    await user.click(within(dialog).getByRole("button", { name: "Pass" }));
    expect(await screen.findByText("44")).toBeInTheDocument();
    expect(screen.getByText("the game changed")).toBeInTheDocument();
  });

  it("asks before leaving an unfinished game", async () => {
    const { user } = await startGame();
    await user.click(screen.getByRole("button", { name: "New game" }));
    const dialog = screen.getByRole("dialog", { name: /new game/i });
    await user.click(within(dialog).getByRole("button", { name: "New game" }));
    expect(await screen.findByText("Choose your opponent")).toBeInTheDocument();
    expect(localStorage.getItem("rackwise.gameId")).toBeNull();
  });
});
