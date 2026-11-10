import { useEffect, useRef } from "react";

import type { Turn } from "../api";

function describe(turn: Turn): string {
  if (turn.kind === "play") return `${turn.word} at ${turn.position}${turn.bingo ? " (bingo)" : ""}`;
  if (turn.kind === "pass") return "Passed";
  const count = turn.exchanged_count ?? 0;
  return turn.exchanged ? `Exchanged ${turn.exchanged}` : `Exchanged ${count} ${count === 1 ? "tile" : "tiles"}`;
}

export function MoveList({ turns }: { turns: Turn[] }) {
  const end = useRef<HTMLLIElement>(null);
  useEffect(() => {
    // jsdom has no scrollIntoView, hence the optional call.
    end.current?.scrollIntoView?.({ block: "nearest" });
  }, [turns.length]);

  if (turns.length === 0) return <p className="muted">No moves yet. You go first.</p>;
  return (
    <ol className="move-list">
      {turns.map((turn, index) => (
        <li key={index} ref={index === turns.length - 1 ? end : undefined} className={`move move-${turn.player}`}>
          <span className="move-player">{turn.player === "you" ? "You" : "Bot"}</span>
          <span className="move-text">{describe(turn)}</span>
          <span className="move-score">+{turn.score}</span>
          <span className="move-total">{turn.total}</span>
        </li>
      ))}
    </ol>
  );
}
