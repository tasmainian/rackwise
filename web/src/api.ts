// The engine's HTTP interface, documented in docs/engine-api.md.

const BASE = "/api/engine/v1";

export type Level = "easy" | "medium" | "hard";

export interface Rules {
  board_size: number;
  rack_size: number;
  center: number;
  bingo_bonus: number;
  min_bag_for_exchange: number;
  premiums: string[];
  tile_values: Record<string, number>;
  levels: Level[];
}

export interface TilePlacement {
  row: number;
  col: number;
  letter: string;
}

export interface Turn {
  player: "you" | "bot";
  kind: "play" | "exchange" | "pass";
  position?: string;
  word?: string;
  cross_words?: string[];
  tiles?: TilePlacement[];
  bingo?: boolean;
  exchanged?: string;
  exchanged_count?: number;
  score: number;
  total: number;
}

export interface Result {
  outcome: "win" | "loss" | "draw";
  reason: "went_out" | "scoreless";
  your_rack: string;
  bot_rack: string;
  adjustments: { you: number; bot: number };
}

export interface GameView {
  id: string;
  level: Level;
  status: "active" | "finished";
  board: string[];
  rack: string;
  scores: { you: number; bot: number };
  bag_remaining: number;
  bot_tiles: number;
  can_exchange: boolean;
  turns: Turn[];
  result?: Result;
}

/** An error reported by the engine. `code` is missing if the engine wasn't reached. */
export class ApiError extends Error {
  readonly status: number;
  readonly code?: string;

  constructor(message: string, status: number, code?: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

async function request<T>(path: string, body?: unknown): Promise<T> {
  let response: Response;
  try {
    response = await fetch(BASE + path, {
      method: body === undefined ? "GET" : "POST",
      headers: body === undefined ? undefined : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new ApiError("Can't reach the game engine. Is it running?", 0);
  }
  const data: unknown = await response.json().catch(() => null);
  if (!response.ok) {
    const failure = (data ?? {}) as { error?: string; code?: string };
    throw new ApiError(failure.error ?? "The game engine reported an error.", response.status, failure.code);
  }
  return data as T;
}

export const api = {
  rules: () => request<Rules>("/rules"),
  createGame: (level: Level) => request<GameView>("/games", { level }),
  getGame: (id: string) => request<GameView>(`/games/${id}`),
  play: (id: string, tiles: TilePlacement[]) => request<GameView>(`/games/${id}/play`, { tiles }),
  exchange: (id: string, tiles: string) => request<GameView>(`/games/${id}/exchange`, { tiles }),
  pass: (id: string) => request<GameView>(`/games/${id}/pass`, {}),
};

export function recordUrl(id: string): string {
  return `${BASE}/games/${id}/gcg`;
}
