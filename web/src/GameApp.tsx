import { useEffect, useState } from "react";

import { api, ApiError, type GameView, type Level, type Rules } from "./api";
import { GameScreen } from "./GameScreen";
import { SetupScreen } from "./SetupScreen";

// Not reachable from the app: the bots were removed from the product at the developer's
// request. Kept, with its tests, until they decide whether to delete it.
// The current game's id survives a page refresh. The game itself is in the engine.
const GAME_KEY = "rackwise.gameId";

async function loadStart(): Promise<{ rules: Rules; game: GameView | null }> {
  const rules = await api.rules();
  const id = localStorage.getItem(GAME_KEY);
  if (id === null) return { rules, game: null };
  try {
    return { rules, game: await api.getGame(id) };
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) {
      localStorage.removeItem(GAME_KEY);
      return { rules, game: null };
    }
    throw error;
  }
}

export function GameApp() {
  const [rules, setRules] = useState<Rules | null>(null);
  const [game, setGame] = useState<GameView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);


  useEffect(() => {
    let ignore = false;
    loadStart()
      .then((start) => {
        if (ignore) return;
        setRules(start.rules);
        setGame(start.game);
      })
      .catch((failure: Error) => {
        if (!ignore) setError(failure.message);
      });
    return () => {
      ignore = true;
    };
  }, []);

  const start = async (level: Level) => {
    setBusy(true);
    setError(null);
    try {
      const created = await api.createGame(level);
      localStorage.setItem(GAME_KEY, created.id);
      setGame(created);
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "Couldn't start a game.");
    } finally {
      setBusy(false);
    }
  };

  const newGame = () => {
    localStorage.removeItem(GAME_KEY);
    setGame(null);
  };

  return (
    <div className="app">
      <header className="app-header">
        <h1>Rackwise</h1>
      </header>
      <main>
        {error && (
          <p className="error" role="alert">
            {error}
          </p>
        )}
        {!rules && !error && <p className="muted">Loading...</p>}
        {rules && !game && <SetupScreen levels={rules.levels} busy={busy} onStart={(level) => void start(level)} />}
        {rules && game && <GameScreen key={game.id} rules={rules} initialView={game} onNewGame={newGame} />}
      </main>
    </div>
  );
}
