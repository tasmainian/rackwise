import type { Level } from "./api";

const DESCRIPTIONS: Record<Level, string> = {
  easy: "Sticks to everyday words and doesn't always find its best play.",
  medium: "Always plays its highest-scoring move.",
  hard: "Weighs the score against the tiles it keeps and the board it opens.",
};

export function SetupScreen({
  levels,
  busy,
  onStart,
}: {
  levels: Level[];
  busy: boolean;
  onStart: (level: Level) => void;
}) {
  return (
    <section className="setup">
      <h2>Choose your opponent</h2>
      <div className="level-list">
        {levels.map((level) => (
          <button key={level} type="button" className="level" disabled={busy} onClick={() => onStart(level)}>
            <span className="level-name">{level}</span>
            <span className="level-description">{DESCRIPTIONS[level]}</span>
          </button>
        ))}
      </div>
    </section>
  );
}
