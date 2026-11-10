import { type ReactNode, useEffect, useRef, useState } from "react";

import { BLANK } from "../placement";

interface ModalProps {
  title: string;
  onClose: () => void;
  children: ReactNode;
}

// The browser's <dialog> handles focus trapping and the Escape key.
function Modal({ title, onClose, children }: ModalProps) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    ref.current?.showModal();
  }, []);
  return (
    <dialog
      ref={ref}
      className="modal"
      aria-label={title}
      onCancel={(event) => {
        event.preventDefault();
        onClose();
      }}
    >
      <h2>{title}</h2>
      {children}
    </dialog>
  );
}

const LETTERS = [..."ABCDEFGHIJKLMNOPQRSTUVWXYZ"];

// Adapted from ably-labs/word-game, src/component/dialog/BlankTileDialog.js.
// Copyright 2022 Ably Real-time Ltd. Licensed under Apache-2.0; see
// third_party/ably-word-game/LICENSE.
// Changed for Rackwise: rewritten in TypeScript on a native <dialog>, without MUI.
export function BlankDialog({ onChoose, onCancel }: { onChoose: (letter: string) => void; onCancel: () => void }) {
  return (
    <Modal title="Choose a letter for your blank" onClose={onCancel}>
      <div className="letter-grid">
        {LETTERS.map((letter) => (
          <button key={letter} type="button" className="letter-choice" onClick={() => onChoose(letter)}>
            {letter}
          </button>
        ))}
      </div>
      <div className="modal-actions">
        <button type="button" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </Modal>
  );
}

export function ExchangeDialog({
  rack,
  onExchange,
  onCancel,
}: {
  rack: string;
  onExchange: (tiles: string) => void;
  onCancel: () => void;
}) {
  const [chosen, setChosen] = useState<number[]>([]);
  const toggle = (index: number) =>
    setChosen((current) => (current.includes(index) ? current.filter((i) => i !== index) : [...current, index]));
  const tiles = chosen.map((index) => rack[index]).join("");

  return (
    <Modal title="Exchange tiles" onClose={onCancel}>
      <p>Choose the tiles to swap. Exchanging uses your turn.</p>
      <div className="letter-grid">
        {[...rack].map((tile, index) => (
          <button
            key={index}
            type="button"
            className={`letter-choice${chosen.includes(index) ? " letter-chosen" : ""}`}
            aria-pressed={chosen.includes(index)}
            onClick={() => toggle(index)}
          >
            {tile === BLANK ? "Blank" : tile}
          </button>
        ))}
      </div>
      <div className="modal-actions">
        <button type="button" onClick={onCancel}>
          Cancel
        </button>
        <button type="button" className="primary" disabled={tiles === ""} onClick={() => onExchange(tiles)}>
          {tiles === "" ? "Exchange" : `Exchange ${tiles.length}`}
        </button>
      </div>
    </Modal>
  );
}

export function ConfirmDialog({
  title,
  body,
  confirmLabel,
  onConfirm,
  onCancel,
}: {
  title: string;
  body: string;
  confirmLabel: string;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  return (
    <Modal title={title} onClose={onCancel}>
      <p>{body}</p>
      <div className="modal-actions">
        <button type="button" onClick={onCancel}>
          Cancel
        </button>
        <button type="button" className="primary" onClick={onConfirm}>
          {confirmLabel}
        </button>
      </div>
    </Modal>
  );
}
