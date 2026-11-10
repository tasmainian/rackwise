import { useEffect, useState } from "react";

import {
  checkPlay,
  loadPremiums,
  readBoard,
  reviewTurn,
  summarizeGame,
  type CoachStatus,
  type Comparison,
  type GameSummary,
  type PlacedTile,
  type PlayCheck,
} from "./coachApi";
import {
  emptyBoard,
  PositionBoard,
  type EntryMode,
} from "./components/PositionBoard";

// A game takes a while to enter, so it survives a refresh.
const GAME_KEY = "rackwise.reviewGame";
const RACK_SIZE = 7;

interface Turn {
  boardBefore: string[];
  rack: string;
  tiles: PlacedTile[];
  // Null for a swap or a pass.
  check: PlayCheck | null;
  comparison?: Comparison;
}

interface SavedGame {
  board: string[];
  turns: Turn[];
  summary: GameSummary | null;
}

function loadGame(): SavedGame {
  const saved = localStorage.getItem(GAME_KEY);
  return saved
    ? (JSON.parse(saved) as SavedGame)
    : { board: emptyBoard(), turns: [], summary: null };
}

// Claude charges for an image by its pixel size, not its file size, so shrinking a picture
// before upload is what makes it cheaper. Letter tiles stay large enough to read at this size.
const MAX_IMAGE_EDGE = 1568;

function fileAsBase64(file: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () =>
      resolve((reader.result as string).split(",")[1] ?? "");
    reader.onerror = () => reject(new Error("That file couldn't be opened."));
    reader.readAsDataURL(file);
  });
}

// Returns the picture as base64, shrunk to MAX_IMAGE_EDGE on its long edge as a JPEG. If the
// browser can't draw it, the original is sent and the coach checks it as before.
async function pictureAsBase64(file: File): Promise<string> {
  try {
    const bitmap = await createImageBitmap(file);
    const scale = Math.min(
      1,
      MAX_IMAGE_EDGE / Math.max(bitmap.width, bitmap.height),
    );
    const canvas = document.createElement("canvas");
    canvas.width = Math.round(bitmap.width * scale);
    canvas.height = Math.round(bitmap.height * scale);
    const context = canvas.getContext("2d");
    if (!context) throw new Error("no canvas");
    context.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
    const shrunk = await new Promise<Blob | null>((resolve) =>
      canvas.toBlob(resolve, "image/jpeg", 0.9),
    );
    if (!shrunk) throw new Error("no image");
    return await fileAsBase64(shrunk);
  } catch {
    return fileAsBase64(file);
  }
}

function changedTiles(before: string[], after: string[]): number {
  let changed = 0;
  before.forEach((line, row) => {
    [...line].forEach((cell, col) => {
      if (cell !== "." && after[row][col] !== cell) changed += 1;
    });
  });
  return changed;
}

function tilesIn(play: string[]): PlacedTile[] {
  const tiles: PlacedTile[] = [];
  play.forEach((line, row) => {
    [...line].forEach((letter, col) => {
      if (letter !== ".") tiles.push({ row, col, letter });
    });
  });
  return tiles;
}

function merged(board: string[], play: string[]): string[] {
  return board.map((line, row) =>
    [...line]
      .map((cell, col) => (play[row][col] !== "." ? play[row][col] : cell))
      .join(""),
  );
}

function playGrid(tiles: PlacedTile[]): string[] {
  const grid = emptyBoard().map((line) => [...line]);
  tiles.forEach((tile) => {
    grid[tile.row][tile.col] = tile.letter;
  });
  return grid.map((line) => line.join(""));
}

const STATUS_MESSAGES: Partial<Record<CoachStatus, string>> = {
  no_api_key:
    "Coaching is off because no ANTHROPIC_API_KEY is set. The turn review below still works.",
  rejected:
    "The coach's answer used a word or number that isn't in the data, so it was withheld.",
  error: "The coach couldn't answer just now. Finish the game again to retry.",
};

function TurnReview({
  turn,
  number,
  summary,
}: {
  turn: Turn;
  number: number;
  summary?: GameSummary;
}) {
  const note = summary?.coaching;
  const comparison = turn.comparison;
  if (!comparison) return null;
  const played = comparison.played;
  const best = comparison.options[0];
  const coachNote = note?.turns.find((item) => item.turn === number);
  return (
    <li>
      <h4>
        Turn {number}:{" "}
        {played ? played.words_formed.join(", ") : "swap or pass"}
        {played?.status === "listed" && ` (${played.play_score})`}
      </h4>
      {played?.status === "not_valid" && (
        <p>Not valid: {played.invalid_words.join(", ")}.</p>
      )}
      {played?.points_behind_best != null && played.points_behind_best > 0 && (
        <p>{played.points_behind_best} behind the best play found.</p>
      )}
      {best && (
        <p className="muted">
          Best found: {best.words_formed.join(", ")} at {best.position} for{" "}
          {best.play_score}, keeping {best.leave || "no tiles"}.
        </p>
      )}
      {comparison.more_available && (
        <p className="muted">
          The search was cut off, so a better play may exist.
        </p>
      )}
      {coachNote && (
        <p>
          <strong>
            Coach
            {coachNote.recommended_word
              ? `: play ${coachNote.recommended_word}`
              : ""}
            .
          </strong>{" "}
          {coachNote.note}
        </p>
      )}
      {coachNote?.cited_notes?.map((id) => {
        const cited = summary?.notes?.[id];
        return (
          cited && (
            <details key={id} className="strategy-note">
              <summary>Strategy note: {cited.title}</summary>
              <p>{cited.body}</p>
            </details>
          )
        );
      })}
    </li>
  );
}

export function PracticeScreen() {
  const [game, setGame] = useState<SavedGame>(loadGame);
  const [play, setPlay] = useState<string[]>(emptyBoard);
  const [premiums, setPremiums] = useState<string[]>([]);
  const [rack, setRack] = useState("");
  const [mode, setMode] = useState<EntryMode>("board");
  const [down, setDown] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState<string | null>(null);
  const [upload, setUpload] = useState<{
    before: string[];
    rackBefore: string;
    message: string;
  } | null>(null);

  useEffect(() => {
    let ignore = false;
    void loadPremiums().then((layout) => {
      if (!ignore) setPremiums(layout);
    });
    return () => {
      ignore = true;
    };
  }, []);

  const save = (next: SavedGame) => {
    setGame(next);
    localStorage.setItem(GAME_KEY, JSON.stringify(next));
  };

  const addTurn = (turn: Turn, board: string[]) => {
    save({ board, turns: [...game.turns, turn], summary: null });
    setPlay(emptyBoard());
    setRack("");
    setMode("board");
  };

  const recordPlay = async () => {
    setBusy(true);
    setError(null);
    const tiles = tilesIn(play);
    try {
      const check = await checkPlay(rack, game.board, tiles);
      addTurn(
        { boardBefore: game.board, rack, tiles, check },
        merged(game.board, play),
      );
    } catch (failure) {
      setError(
        failure instanceof Error
          ? failure.message
          : "That play couldn't be recorded.",
      );
    } finally {
      setBusy(false);
    }
  };

  const recordSwap = () => {
    setError(null);
    if (rack === "") {
      setError(
        "Enter your rack, so the review can show what you could have played.",
      );
      return;
    }
    addTurn(
      { boardBefore: game.board, rack, tiles: [], check: null },
      game.board,
    );
  };

  const undo = () => {
    const last = game.turns.at(-1);
    if (!last) return;
    save({
      board: last.boardBefore,
      turns: game.turns.slice(0, -1),
      summary: null,
    });
    setPlay(playGrid(last.tiles));
    setRack(last.rack);
    setMode("play");
    setError(null);
  };

  const finish = async () => {
    setBusy(true);
    setError(null);
    const turns = [...game.turns];
    try {
      // Reviews go one at a time, and each is saved, so a failure doesn't repeat finished work.
      for (let i = 0; i < turns.length; i++) {
        if (!turns[i].comparison) {
          setProgress(`Reviewing turn ${i + 1} of ${turns.length}...`);
          const comparison = await reviewTurn(
            turns[i].rack,
            turns[i].boardBefore,
            turns[i].tiles,
          );
          turns[i] = { ...turns[i], comparison };
          save({ ...game, turns: [...turns], summary: null });
        }
      }
      setProgress("Writing your coaching summary...");
      const summary = await summarizeGame(
        turns.map((turn, i) => ({
          ...(turn.comparison as Comparison),
          turn: i + 1,
        })),
      );
      save({ ...game, turns, summary });
    } catch (failure) {
      setError(
        failure instanceof Error ? failure.message : "The review failed.",
      );
    } finally {
      setBusy(false);
      setProgress(null);
    }
  };

  const uploadPicture = async (file: File) => {
    setBusy(true);
    setError(null);
    setUpload(null);
    setProgress("Reading your picture...");
    try {
      const reading = await readBoard(await pictureAsBase64(file));
      const count = reading.rows.join("").replace(/\./g, "").length;
      const changed = changedTiles(game.board, reading.rows);
      const warning =
        changed > 0
          ? ` ${changed} tiles you already had were changed or removed.`
          : "";
      setUpload({
        before: game.board,
        rackBefore: rack,
        message: `Read ${count} tiles. Check them against your game, and type over any that are wrong.${warning}`,
      });
      save({ ...game, board: reading.rows, summary: null });
      setPlay(emptyBoard());
      if (reading.rack) setRack(reading.rack);
    } catch (failure) {
      setError(
        failure instanceof Error
          ? failure.message
          : "That picture couldn't be read.",
      );
    } finally {
      setBusy(false);
      setProgress(null);
    }
  };

  const undoUpload = () => {
    if (!upload) return;
    save({ ...game, board: upload.before, summary: null });
    setRack(upload.rackBefore);
    setUpload(null);
  };

  const newGame = () => {
    if (
      game.turns.length > 0 &&
      !window.confirm("Start a new game? This game's turns will be deleted.")
    )
      return;
    save({ board: emptyBoard(), turns: [], summary: null });
    setPlay(emptyBoard());
    setRack("");
    setMode("board");
    setError(null);
  };

  const summary = game.summary;
  const message = summary ? STATUS_MESSAGES[summary.coach_status] : undefined;

  return (
    <section className="practice">
      <h2>Track your game</h2>
      <p className="muted">
        Upload a picture of your game's board, or type the tiles in. On your
        turn, switch to My play, enter your rack and place your tiles. Green
        tiles are this turn's play.
      </p>

      <div className="position-upload">
        <label>
          Upload a picture of your board
          <input
            type="file"
            accept="image/png,image/jpeg,image/gif,image/webp"
            disabled={busy}
            onChange={(event) => {
              const file = event.target.files?.[0];
              event.target.value = "";
              if (file) void uploadPicture(file);
            }}
          />
        </label>
        {upload && (
          <p role="status">
            {upload.message}{" "}
            <button type="button" onClick={undoUpload}>
              Undo upload
            </button>
          </p>
        )}
      </div>

      <div className="position-tools">
        <div
          role="group"
          aria-label="What you're entering"
          className="position-modes"
        >
          <button
            type="button"
            aria-pressed={mode === "board"}
            onClick={() => setMode("board")}
          >
            Tiles on the board
          </button>
          <button
            type="button"
            aria-pressed={mode === "play"}
            onClick={() => setMode("play")}
          >
            My play
          </button>
        </div>
        <button type="button" onClick={() => setDown(!down)}>
          Typing: {down ? "down" : "across"}
        </button>
      </div>

      <PositionBoard
        board={game.board}
        play={play}
        premiums={premiums}
        mode={mode}
        down={down}
        onChange={(grid) => {
          if (mode === "play") setPlay(grid);
          else save({ ...game, board: grid, summary: null });
        }}
      />

      <div className="practice-form">
        <label>
          Your rack (use ? for a blank)
          <input
            value={rack}
            maxLength={RACK_SIZE}
            onChange={(event) => setRack(event.target.value.toUpperCase())}
          />
        </label>
        <div className="position-tools">
          <button
            type="button"
            className="primary"
            disabled={busy}
            onClick={() => void recordPlay()}
          >
            Record my play
          </button>
          <button type="button" disabled={busy} onClick={recordSwap}>
            I swapped or passed
          </button>
        </div>
      </div>

      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}

      {game.turns.length > 0 && (
        <>
          <h3>Your turns</h3>
          <table className="practice-table">
            <thead>
              <tr>
                <th scope="col">Turn</th>
                <th scope="col">Rack</th>
                <th scope="col">Words formed</th>
                <th scope="col">Position</th>
                <th scope="col">Score</th>
              </tr>
            </thead>
            <tbody>
              {game.turns.map((turn, index) => (
                <tr key={index}>
                  <th scope="row">{index + 1}</th>
                  <td>{turn.rack}</td>
                  <td>
                    {turn.check
                      ? turn.check.words_formed.join(", ")
                      : "Swap or pass"}
                  </td>
                  <td>{turn.check?.position ?? "None"}</td>
                  <td>{turn.check?.play_score ?? 0}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="position-tools">
            <button
              type="button"
              className="primary"
              disabled={busy}
              onClick={() => void finish()}
            >
              Finish game and get coaching
            </button>
            <button type="button" disabled={busy} onClick={undo}>
              Undo last turn
            </button>
          </div>
        </>
      )}
      <div className="position-tools">
        <button type="button" disabled={busy} onClick={newGame}>
          New game
        </button>
      </div>
      {progress && (
        <p className="muted" role="status">
          {progress}
        </p>
      )}

      {summary && (
        <section aria-label="Game summary" className="practice-results">
          <h3>Game summary</h3>
          <p>
            You scored <strong>{summary.stats.total_score}</strong> and were{" "}
            <strong>{summary.stats.total_points_behind}</strong> behind the best
            plays found, or {summary.stats.average_points_behind} a turn.
            {summary.stats.biggest_miss_turn !== null &&
              summary.stats.biggest_miss_points > 0 &&
              ` Your biggest miss was ${summary.stats.biggest_miss_points} on turn ${summary.stats.biggest_miss_turn}.`}
          </p>
          <p className="muted">
            Scores use word.tips' word list and tile values with your board's
            multipliers, and a 40-point bonus for using all seven tiles.
          </p>
          {summary.coaching && (
            <>
              <p>{summary.coaching.summary}</p>
              {summary.coaching.themes.length > 0 && (
                <ul>
                  {summary.coaching.themes.map((theme) => (
                    <li key={theme}>{theme}</li>
                  ))}
                </ul>
              )}
            </>
          )}
          {message && <p className="muted">{message}</p>}
          <ol className="turn-reviews">
            {game.turns.map((turn, index) => (
              <TurnReview
                key={index}
                turn={turn}
                number={index + 1}
                summary={summary}
              />
            ))}
          </ol>
        </section>
      )}
    </section>
  );
}
