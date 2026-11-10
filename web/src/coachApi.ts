export type CoachStatus = "ok" | "no_api_key" | "rejected" | "error";

export interface PlacedTile {
  row: number;
  col: number;
  letter: string;
}

export interface PlayCheck {
  words_formed: string[];
  position: string;
  play_score: number;
}

export interface PracticeOption {
  word: string;
  position: string;
  words_formed: string[];
  play_score: number;
  leave: string;
}

export interface PlayedWord extends PracticeOption {
  status: "listed" | "not_valid";
  invalid_words: string[];
  points_behind_best: number | null;
}

export interface Comparison {
  rack: string;
  more_available: boolean;
  options: PracticeOption[];
  played: PlayedWord | null;
}

export interface GameStats {
  turns: number;
  swaps_or_passes: number;
  invalid_plays: number;
  total_score: number;
  total_points_behind: number;
  average_points_behind: number;
  biggest_miss_points: number;
  biggest_miss_turn: number | null;
  all_tile_plays: number;
}

export interface GameSummary {
  stats: GameStats;
  coach_status: CoachStatus;
  coaching: {
    summary: string;
    themes: string[];
    // cited_notes is missing from summaries saved before strategy notes existed.
    turns: {
      turn: number;
      recommended_word: string | null;
      note: string;
      cited_notes?: string[];
    }[];
  } | null;
  /** The full text of each strategy note the coach cited, keyed by note ID. */
  notes?: Record<string, { title: string; body: string }>;
}

async function post<T>(path: string, body: unknown): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`/api/coach/v1/practice/${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  } catch {
    throw new Error("Can't reach the coach. Is it running?");
  }
  const data: unknown = await response.json().catch(() => null);
  if (!response.ok) {
    throw new Error(
      (data as { error?: string } | null)?.error ?? "The request failed.",
    );
  }
  return data as T;
}

// T is 3W, D is 2W, t is 3L and d is 2L. The coach defines the layout. If it can't be
// reached, the board still works without colours.
export async function loadPremiums(): Promise<string[]> {
  try {
    const response = await fetch("/api/coach/v1/practice/premiums");
    const data = (await response.json()) as { premiums?: string[] };
    return data.premiums ?? [];
  } catch {
    return [];
  }
}

export function readBoard(
  imageBase64: string,
): Promise<{ rows: string[]; rack: string }> {
  return post("read-board", { image_base64: imageBase64 });
}

export function checkPlay(
  rack: string,
  board: string[],
  tiles: PlacedTile[],
): Promise<PlayCheck> {
  return post("check-play", { rack, board, tiles });
}

export function reviewTurn(
  rack: string,
  board: string[],
  tiles: PlacedTile[],
): Promise<Comparison> {
  return post("review", { rack, board, tiles });
}

export function summarizeGame(
  turns: (Comparison & { turn: number })[],
): Promise<GameSummary> {
  return post("summary", { turns });
}
