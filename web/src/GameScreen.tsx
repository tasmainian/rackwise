import { useMemo, useState } from "react";

import { api, ApiError, type GameView, recordUrl, type Rules } from "./api";
import { Board } from "./components/Board";
import { BlankDialog, ConfirmDialog, ExchangeDialog } from "./components/Dialogs";
import { MoveList } from "./components/MoveList";
import { DRAG_TYPE, Tile } from "./components/Tile";
import {
  BLANK,
  isSquareFree,
  type PendingTile,
  pendingAt,
  placeTile,
  type RackTile,
  removeTile,
  shuffled,
  tilesOnRack,
  toPlacements,
  toRackTiles,
} from "./placement";

type Dialog =
  | { kind: "blank"; tileId: number; row: number; col: number }
  | { kind: "exchange" }
  | { kind: "pass" }
  | { kind: "newGame" }
  | null;

const OUTCOMES = { win: "You won!", loss: "You lost.", draw: "It's a draw." };

export function GameScreen({
  rules,
  initialView,
  onNewGame,
}: {
  rules: Rules;
  initialView: GameView;
  onNewGame: () => void;
}) {
  const [view, setView] = useState(initialView);
  const [rack, setRack] = useState<RackTile[]>(() => toRackTiles(initialView.rack));
  const [pending, setPending] = useState<PendingTile[]>([]);
  const [selected, setSelected] = useState<number | null>(null);
  const [dialog, setDialog] = useState<Dialog>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);

  const finished = view.status === "finished";
  const locked = finished || busy;

  // The opponent's last play gets a highlight, so it's easy to see what changed.
  const highlighted = useMemo(() => {
    const last = view.turns.findLast((turn) => turn.player === "bot");
    return new Set((last?.tiles ?? []).map((tile) => `${tile.row},${tile.col}`));
  }, [view.turns]);

  const applyView = (next: GameView) => {
    setView(next);
    setRack(toRackTiles(next.rack));
    setPending([]);
    setSelected(null);
  };

  const send = async (action: () => Promise<GameView>) => {
    setBusy(true);
    setMessage(null);
    setDialog(null);
    try {
      applyView(await action());
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Something went wrong.");
      // The game moved on elsewhere, such as in another tab, so show its real state.
      if (error instanceof ApiError && (error.code === "conflict" || error.code === "game_over")) {
        applyView(await api.getGame(view.id).catch(() => view));
      }
    } finally {
      setBusy(false);
    }
  };

  const place = (tileId: number, row: number, col: number) => {
    const tile = rack.find((candidate) => candidate.id === tileId);
    if (!tile || locked || !isSquareFree(view.board, pending, row, col)) return;
    setSelected(null);
    setMessage(null);
    if (tile.tile === BLANK) {
      // A blank being moved keeps its letter.
      const placed = pending.find((other) => other.tileId === tileId);
      if (placed) {
        setPending(placeTile(pending, { ...placed, row, col }));
      } else {
        setDialog({ kind: "blank", tileId, row, col });
      }
      return;
    }
    setPending(placeTile(pending, { tileId, row, col, letter: tile.tile, blank: false }));
  };

  const onSquareClick = (row: number, col: number) => {
    if (selected !== null) {
      place(selected, row, col);
      return;
    }
    const placed = pendingAt(pending, row, col);
    if (placed) setPending(removeTile(pending, placed.tileId));
  };

  const recall = () => {
    setPending([]);
    setSelected(null);
  };

  const rackLeft = tilesOnRack(rack, pending);

  return (
    <div className="game">
      <div className="game-main">
        <Board
          rules={rules}
          board={view.board}
          pending={pending}
          highlighted={highlighted}
          disabled={locked}
          onSquareClick={onSquareClick}
          onTileDrop={place}
        />

        {!finished && (
          <div
            className="rack"
            aria-label="Your rack"
            role="group"
            onDragOver={(event) => event.preventDefault()}
            onDrop={(event) => {
              event.preventDefault();
              const tileId = event.dataTransfer.getData(DRAG_TYPE);
              if (tileId !== "") setPending(removeTile(pending, Number(tileId)));
            }}
          >
            {rackLeft.map((tile) => (
              <button
                key={tile.id}
                type="button"
                className={`rack-tile${selected === tile.id ? " rack-tile-selected" : ""}`}
                aria-label={tile.tile === BLANK ? "Blank tile" : `Tile ${tile.tile}`}
                aria-pressed={selected === tile.id}
                disabled={locked}
                onClick={() => setSelected(selected === tile.id ? null : tile.id)}
              >
                <Tile
                  letter={tile.tile === BLANK ? "" : tile.tile}
                  value={tile.tile === BLANK ? 0 : rules.tile_values[tile.tile]}
                  blank={tile.tile === BLANK}
                  tone="rack"
                  dragId={locked ? undefined : tile.id}
                />
              </button>
            ))}
            {rackLeft.length === 0 && <span className="muted">All tiles placed</span>}
          </div>
        )}

        <p className="status" role="status">
          {message ?? (busy ? "The bot is thinking..." : !finished ? "Click a tile, then a square, or drag it." : "")}
        </p>

        {!finished && (
          <div className="actions">
            <button
              type="button"
              className="primary"
              disabled={locked || pending.length === 0}
              onClick={() => void send(() => api.play(view.id, toPlacements(pending)))}
            >
              Submit
            </button>
            <button type="button" disabled={locked || pending.length === 0} onClick={recall}>
              Recall
            </button>
            <button type="button" disabled={locked} onClick={() => setRack(shuffled(rack))}>
              Shuffle
            </button>
            <button
              type="button"
              disabled={locked || !view.can_exchange}
              title={view.can_exchange ? undefined : `Needs ${rules.min_bag_for_exchange} tiles in the bag`}
              onClick={() => {
                recall();
                setDialog({ kind: "exchange" });
              }}
            >
              Exchange
            </button>
            <button
              type="button"
              disabled={locked}
              onClick={() => {
                recall();
                setDialog({ kind: "pass" });
              }}
            >
              Pass
            </button>
          </div>
        )}
      </div>

      <aside className="game-side">
        <section className="scores" aria-label="Scores">
          <div className="score">
            <span className="score-name">You</span>
            <span className="score-value">{view.scores.you}</span>
          </div>
          <div className="score">
            <span className="score-name">Bot ({view.level})</span>
            <span className="score-value">{view.scores.bot}</span>
          </div>
        </section>
        <p className="muted bag-count">
          {view.bag_remaining} {view.bag_remaining === 1 ? "tile" : "tiles"} in the bag
        </p>

        {view.result && (
          <section className="result">
            <h2>{OUTCOMES[view.result.outcome]}</h2>
            <p>
              {view.result.reason === "went_out"
                ? view.result.your_rack === ""
                  ? "You used all your tiles."
                  : "The bot used all its tiles."
                : "Six turns passed without a score."}
            </p>
            <p className="muted">
              Tiles left: you {view.result.your_rack || "none"}, bot {view.result.bot_rack || "none"}. End adjustment: you{" "}
              {formatPoints(view.result.adjustments.you)}, bot {formatPoints(view.result.adjustments.bot)}.
            </p>
            <a className="button" href={recordUrl(view.id)} download>
              Download game record (GCG)
            </a>
          </section>
        )}

        <button type="button" onClick={() => (finished ? onNewGame() : setDialog({ kind: "newGame" }))}>
          New game
        </button>

        <h2 className="moves-heading">Moves</h2>
        <MoveList turns={view.turns} />
      </aside>

      {dialog?.kind === "blank" && (
        <BlankDialog
          onCancel={() => setDialog(null)}
          onChoose={(letter) => {
            setPending(placeTile(pending, { tileId: dialog.tileId, row: dialog.row, col: dialog.col, letter, blank: true }));
            setDialog(null);
          }}
        />
      )}
      {dialog?.kind === "exchange" && (
        <ExchangeDialog
          rack={view.rack}
          onCancel={() => setDialog(null)}
          onExchange={(tiles) => void send(() => api.exchange(view.id, tiles))}
        />
      )}
      {dialog?.kind === "pass" && (
        <ConfirmDialog
          title="Pass your turn?"
          body="You won't play or exchange any tiles this turn."
          confirmLabel="Pass"
          onCancel={() => setDialog(null)}
          onConfirm={() => void send(() => api.pass(view.id))}
        />
      )}
      {dialog?.kind === "newGame" && (
        <ConfirmDialog
          title="Start a new game?"
          body="You'll leave this game unfinished, and it won't be reviewed."
          confirmLabel="New game"
          onCancel={() => setDialog(null)}
          onConfirm={onNewGame}
        />
      )}
    </div>
  );
}

function formatPoints(points: number): string {
  return points > 0 ? `+${points}` : String(points);
}
