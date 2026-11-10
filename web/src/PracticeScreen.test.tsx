import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { PracticeScreen } from "./PracticeScreen";

const premiums = Array<string>(15).fill(".".repeat(15));
premiums[0] = "T" + ".".repeat(13) + "d";

const check = {
  words_formed: ["ES", "AE", "TS"],
  position: "9H",
  play_score: 6,
};
const option = {
  word: "NUDGE",
  position: "8D",
  words_formed: ["NUDGE"],
  play_score: 24,
  leave: "TU",
};
const comparison = {
  rack: "ESDRINU",
  more_available: false,
  options: [option],
  played: {
    ...option,
    word: "ES",
    position: "9H",
    words_formed: ["ES", "AE", "TS"],
    play_score: 6,
    leave: "DINRU",
    status: "listed",
    invalid_words: [],
    points_behind_best: 18,
  },
};
const stats = {
  turns: 1,
  swaps_or_passes: 0,
  invalid_plays: 0,
  total_score: 6,
  total_points_behind: 18,
  average_points_behind: 18,
  biggest_miss_points: 18,
  biggest_miss_turn: 1,
  all_tile_plays: 0,
};

type Routes = Record<string, { status?: number; body: unknown }>;

function route(routes: Routes) {
  const fetchMock = vi.fn((url: string) => {
    if (url.endsWith("/premiums"))
      return Promise.resolve(new Response(JSON.stringify({ premiums })));
    const match = Object.keys(routes).find((path) => url.endsWith(path));
    const reply = match ? routes[match] : { status: 500, body: {} };
    return Promise.resolve(
      new Response(JSON.stringify(reply.body), { status: reply.status ?? 200 }),
    );
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function bodyOf(fetchMock: ReturnType<typeof route>, path: string): unknown {
  const call = fetchMock.mock.calls.find(([url]) =>
    url.endsWith(path),
  ) as unknown as [string, RequestInit];
  return JSON.parse(call[1].body as string);
}

// Enters CAT on the board, then a stacked play under it.
async function recordStackedPlay(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByLabelText("Square 8G"));
  await user.keyboard("cat");
  await user.click(screen.getByRole("button", { name: "My play" }));
  await user.type(screen.getByLabelText(/your rack/i), "esdrinu");
  await user.click(screen.getByLabelText("Square 9H"));
  await user.keyboard("es");
  await user.click(screen.getByRole("button", { name: "Record my play" }));
}

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
});

describe("PracticeScreen", () => {
  it("records a placed play, listing every word formed with commas", async () => {
    const fetchMock = route({ "/check-play": { body: check } });
    const user = userEvent.setup();
    render(<PracticeScreen />);

    await recordStackedPlay(user);

    expect(
      await screen.findByRole("cell", { name: "ES, AE, TS" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("cell", { name: "9H" })).toBeInTheDocument();
    expect(screen.getByRole("cell", { name: "6" })).toBeInTheDocument();

    const sent = bodyOf(fetchMock, "/check-play") as {
      rack: string;
      board: string[];
      tiles: unknown[];
    };
    expect(sent.rack).toBe("ESDRINU");
    expect(sent.board[7]).toBe("......CAT......");
    expect(sent.board[8]).toBe(".".repeat(15));
    expect(sent.tiles).toEqual([
      { row: 8, col: 7, letter: "E" },
      { row: 8, col: 8, letter: "S" },
    ]);

    // The play joins the board, ready for the next turn.
    expect(screen.getByLabelText("Square 9H")).toHaveClass(
      "position-cell-filled",
    );
    expect(screen.getByLabelText(/your rack/i)).toHaveValue("");
  });

  it("does no coaching or review until the game is finished", async () => {
    const fetchMock = route({ "/check-play": { body: check } });
    const user = userEvent.setup();
    render(<PracticeScreen />);

    await recordStackedPlay(user);
    await screen.findByRole("cell", { name: "ES, AE, TS" });

    const paths = fetchMock.mock.calls.map(([url]) => url);
    expect(
      paths.some((url) => url.endsWith("/review") || url.endsWith("/summary")),
    ).toBe(false);
  });

  it("explains an illegal play and keeps the tiles", async () => {
    route({
      "/check-play": {
        status: 422,
        body: { error: "your play must connect to the tiles on the board" },
      },
    });
    const user = userEvent.setup();
    render(<PracticeScreen />);

    await recordStackedPlay(user);

    expect(await screen.findByRole("alert")).toHaveTextContent("must connect");
    expect(screen.getByLabelText("Square 9H")).toHaveClass(
      "position-cell-play",
    );
    expect(
      screen.queryByRole("cell", { name: "ES, AE, TS" }),
    ).not.toBeInTheDocument();
  });

  it("types past board tiles in My play without changing them", async () => {
    route({});
    const user = userEvent.setup();
    render(<PracticeScreen />);
    await user.click(screen.getByLabelText("Square 8G"));
    await user.keyboard("cat");
    await user.click(screen.getByRole("button", { name: "My play" }));
    await user.click(screen.getByLabelText("Square 8I"));
    await user.keyboard("ts");

    expect(screen.getByLabelText("Square 8I")).toHaveValue("T");
    expect(screen.getByLabelText("Square 8I")).toHaveClass(
      "position-cell-filled",
    );
    expect(screen.getByLabelText("Square 8J")).toHaveValue("S");
    expect(screen.getByLabelText("Square 8J")).toHaveClass(
      "position-cell-play",
    );
  });

  it("reviews every turn, then writes one summary when the game is finished", async () => {
    const fetchMock = route({
      "/check-play": { body: check },
      "/review": { body: comparison },
      "/summary": {
        body: {
          stats,
          coach_status: "ok",
          coaching: {
            summary: "You played a stacked word.",
            themes: ["Look for multipliers."],
            turns: [
              {
                turn: 1,
                recommended_word: "NUDGE",
                note: "NUDGE scores far more.",
                cited_notes: ["N02"],
              },
            ],
          },
          notes: {
            N02: {
              title: "Doubled vowels hurt",
              body: "Two U's crowd out consonants.",
            },
          },
        },
      },
    });
    const user = userEvent.setup();
    render(<PracticeScreen />);
    await recordStackedPlay(user);
    await screen.findByRole("cell", { name: "ES, AE, TS" });

    await user.click(
      screen.getByRole("button", { name: "Finish game and get coaching" }),
    );

    const results = await screen.findByRole("region", { name: "Game summary" });
    expect(
      within(results).getByText("You played a stacked word."),
    ).toBeInTheDocument();
    expect(
      within(results).getByText("Look for multipliers."),
    ).toBeInTheDocument();
    expect(within(results).getByText(/Coach: play NUDGE/)).toBeInTheDocument();
    expect(
      within(results).getByText("Turn 1: ES, AE, TS (6)"),
    ).toBeInTheDocument();
    expect(
      within(results).getByText("18 behind the best play found."),
    ).toBeInTheDocument();
    expect(
      within(results).getByText(/Best found: NUDGE at 8D for 24, keeping TU/),
    ).toBeInTheDocument();
    expect(
      within(results).getByText("Strategy note: Doubled vowels hurt"),
    ).toBeInTheDocument();
    expect(
      within(results).getByText("Two U's crowd out consonants."),
    ).toBeInTheDocument();

    const reviewed = bodyOf(fetchMock, "/review") as {
      board: string[];
      tiles: unknown[];
    };
    expect(reviewed.board[7]).toBe("......CAT......");
    expect(reviewed.tiles).toHaveLength(2);
    const summarized = bodyOf(fetchMock, "/summary") as {
      turns: { turn: number }[];
    };
    expect(summarized.turns[0].turn).toBe(1);
    expect(
      fetchMock.mock.calls.filter(([url]) => url.endsWith("/summary")),
    ).toHaveLength(1);
  });

  it("still shows the review when there is no API key", async () => {
    route({
      "/check-play": { body: check },
      "/review": { body: comparison },
      "/summary": {
        body: { stats, coach_status: "no_api_key", coaching: null },
      },
    });
    const user = userEvent.setup();
    render(<PracticeScreen />);
    await recordStackedPlay(user);
    await screen.findByRole("cell", { name: "ES, AE, TS" });

    await user.click(
      screen.getByRole("button", { name: "Finish game and get coaching" }),
    );

    const results = await screen.findByRole("region", { name: "Game summary" });
    expect(
      within(results).getByText(/no ANTHROPIC_API_KEY/),
    ).toBeInTheDocument();
    expect(
      within(results).getByText("Turn 1: ES, AE, TS (6)"),
    ).toBeInTheDocument();
  });

  it("reports an invalid word in the review", async () => {
    const played = {
      ...comparison.played,
      status: "not_valid",
      invalid_words: ["TS"],
      points_behind_best: null,
    };
    route({
      "/check-play": { body: check },
      "/review": { body: { ...comparison, played } },
      "/summary": {
        body: { stats, coach_status: "no_api_key", coaching: null },
      },
    });
    const user = userEvent.setup();
    render(<PracticeScreen />);
    await recordStackedPlay(user);
    await screen.findByRole("cell", { name: "ES, AE, TS" });

    await user.click(
      screen.getByRole("button", { name: "Finish game and get coaching" }),
    );

    expect(await screen.findByText("Not valid: TS.")).toBeInTheDocument();
  });

  it("keeps finished reviews when one fails, and doesn't repeat them", async () => {
    const fetchMock = route({
      "/check-play": { body: check },
      "/review": { status: 502, body: { error: "site down" } },
    });
    const user = userEvent.setup();
    render(<PracticeScreen />);
    await recordStackedPlay(user);
    await screen.findByRole("cell", { name: "ES, AE, TS" });

    await user.click(
      screen.getByRole("button", { name: "Finish game and get coaching" }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent("site down");
    expect(
      fetchMock.mock.calls.filter(([url]) => url.endsWith("/summary")),
    ).toHaveLength(0);
  });

  it("records a swap or pass, and needs the rack for it", async () => {
    route({});
    const user = userEvent.setup();
    render(<PracticeScreen />);

    await user.click(
      screen.getByRole("button", { name: "I swapped or passed" }),
    );
    expect(screen.getByRole("alert")).toHaveTextContent("Enter your rack");

    await user.type(screen.getByLabelText(/your rack/i), "qqq");
    await user.click(
      screen.getByRole("button", { name: "I swapped or passed" }),
    );
    expect(
      screen.getByRole("cell", { name: "Swap or pass" }),
    ).toBeInTheDocument();
  });

  it("undoes the last turn, restoring its tiles and rack", async () => {
    route({ "/check-play": { body: check } });
    const user = userEvent.setup();
    render(<PracticeScreen />);
    await recordStackedPlay(user);
    await screen.findByRole("cell", { name: "ES, AE, TS" });

    await user.click(screen.getByRole("button", { name: "Undo last turn" }));

    expect(
      screen.queryByRole("cell", { name: "ES, AE, TS" }),
    ).not.toBeInTheDocument();
    expect(screen.getByLabelText("Square 9H")).toHaveClass(
      "position-cell-play",
    );
    expect(screen.getByLabelText(/your rack/i)).toHaveValue("ESDRINU");
  });

  it("keeps the game after a refresh, and only a confirmed new game clears it", async () => {
    route({ "/check-play": { body: check } });
    const user = userEvent.setup();
    const { unmount } = render(<PracticeScreen />);
    await recordStackedPlay(user);
    await screen.findByRole("cell", { name: "ES, AE, TS" });
    unmount();

    render(<PracticeScreen />);
    expect(
      screen.getByRole("cell", { name: "ES, AE, TS" }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Square 8G")).toHaveValue("C");

    vi.stubGlobal(
      "confirm",
      vi.fn(() => false),
    );
    await user.click(screen.getByRole("button", { name: "New game" }));
    expect(
      screen.getByRole("cell", { name: "ES, AE, TS" }),
    ).toBeInTheDocument();

    vi.stubGlobal(
      "confirm",
      vi.fn(() => true),
    );
    await user.click(screen.getByRole("button", { name: "New game" }));
    expect(
      screen.queryByRole("cell", { name: "ES, AE, TS" }),
    ).not.toBeInTheDocument();
    expect(screen.getByLabelText("Square 8G")).toHaveValue("");
  });

  it("shows the multipliers until a tile covers them", async () => {
    route({});
    const user = userEvent.setup();
    render(<PracticeScreen />);

    const square = await screen.findByLabelText("Square 1A (3W)");
    expect(square).toHaveAttribute("placeholder", "3W");
    expect(square).toHaveClass("square-tw");
    expect(screen.getByLabelText("Square 1O (2L)")).toHaveClass("square-dl");

    await user.click(square);
    await user.keyboard("q");
    expect(square).toHaveValue("Q");
    expect(square).toHaveClass("position-cell-filled");
  });

  it("marks the centre square until a tile covers it", async () => {
    route({});
    const user = userEvent.setup();
    render(<PracticeScreen />);

    const centre = screen.getByLabelText("Square 8H (centre)");
    expect(centre).toHaveClass("position-cell-center");
    expect(screen.getByLabelText("Square 8G")).not.toHaveClass(
      "position-cell-center",
    );

    await user.click(centre);
    await user.keyboard("a");
    expect(centre).toHaveClass("position-cell-filled");
    expect(centre).not.toHaveClass("position-cell-center");
  });

  const picture = () =>
    new File(["fake-image"], "board.png", { type: "image/png" });
  const readRows = [
    ...Array<string>(7).fill(".".repeat(15)),
    "......CAT......",
    ...Array<string>(7).fill(".".repeat(15)),
  ];

  it("fills the grid and rack from a picture, and asks the player to check", async () => {
    const fetchMock = route({
      "/read-board": { body: { rows: readRows, rack: "ESDRINU" } },
    });
    const user = userEvent.setup();
    render(<PracticeScreen />);

    await user.upload(screen.getByLabelText(/upload a picture/i), picture());

    expect(await screen.findByText(/Read 3 tiles/)).toBeInTheDocument();
    expect(screen.getByLabelText("Square 8G")).toHaveValue("C");
    expect(screen.getByLabelText("Square 8G")).toHaveClass(
      "position-cell-filled",
    );
    expect(screen.getByLabelText(/your rack/i)).toHaveValue("ESDRINU");
    const sent = bodyOf(fetchMock, "/read-board") as { image_base64: string };
    expect(atob(sent.image_base64)).toBe("fake-image");
  });

  it("shrinks a large picture to 1568 pixels on its long edge before sending it", async () => {
    const fetchMock = route({
      "/read-board": { body: { rows: readRows, rack: "" } },
    });
    const drawImage = vi.fn();
    vi.stubGlobal(
      "createImageBitmap",
      vi.fn(() => Promise.resolve({ width: 1290, height: 2796 })),
    );
    const getContext = vi
      .spyOn(HTMLCanvasElement.prototype, "getContext")
      .mockReturnValue({
        drawImage,
      } as unknown as CanvasRenderingContext2D);
    const toBlob = vi
      .spyOn(HTMLCanvasElement.prototype, "toBlob")
      .mockImplementation((callback, type) =>
        callback(new Blob(["small-jpeg"], { type })),
      );
    const user = userEvent.setup();
    render(<PracticeScreen />);

    await user.upload(screen.getByLabelText(/upload a picture/i), picture());

    expect(await screen.findByText(/Read 3 tiles/)).toBeInTheDocument();
    expect(drawImage).toHaveBeenCalledWith(expect.anything(), 0, 0, 723, 1568);
    expect(toBlob).toHaveBeenCalledWith(
      expect.any(Function),
      "image/jpeg",
      0.9,
    );
    const sent = bodyOf(fetchMock, "/read-board") as { image_base64: string };
    expect(atob(sent.image_base64)).toBe("small-jpeg");
    getContext.mockRestore();
    toBlob.mockRestore();
  });

  it("leaves a small picture at its own size", async () => {
    route({ "/read-board": { body: { rows: readRows, rack: "" } } });
    const drawImage = vi.fn();
    vi.stubGlobal(
      "createImageBitmap",
      vi.fn(() => Promise.resolve({ width: 800, height: 600 })),
    );
    const getContext = vi
      .spyOn(HTMLCanvasElement.prototype, "getContext")
      .mockReturnValue({
        drawImage,
      } as unknown as CanvasRenderingContext2D);
    const toBlob = vi
      .spyOn(HTMLCanvasElement.prototype, "toBlob")
      .mockImplementation((callback, type) =>
        callback(new Blob(["jpeg"], { type })),
      );
    const user = userEvent.setup();
    render(<PracticeScreen />);

    await user.upload(screen.getByLabelText(/upload a picture/i), picture());

    await screen.findByText(/Read 3 tiles/);
    expect(drawImage).toHaveBeenCalledWith(expect.anything(), 0, 0, 800, 600);
    getContext.mockRestore();
    toBlob.mockRestore();
  });

  it("warns about changed tiles, and can undo an upload", async () => {
    route({ "/read-board": { body: { rows: readRows, rack: "" } } });
    const user = userEvent.setup();
    render(<PracticeScreen />);
    await user.click(screen.getByLabelText("Square 1B"));
    await user.keyboard("q");
    await user.type(screen.getByLabelText(/your rack/i), "abc");

    await user.upload(screen.getByLabelText(/upload a picture/i), picture());

    expect(
      await screen.findByText(/1 tiles you already had/),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Square 1B")).toHaveValue("");
    expect(screen.getByLabelText(/your rack/i)).toHaveValue("ABC");

    await user.click(screen.getByRole("button", { name: "Undo upload" }));
    expect(screen.getByLabelText("Square 1B")).toHaveValue("Q");
    expect(screen.getByLabelText("Square 8G")).toHaveValue("");
  });

  it("shows why a picture couldn't be read, and leaves the board alone", async () => {
    route({
      "/read-board": {
        status: 503,
        body: { error: "Reading a picture needs an ANTHROPIC_API_KEY." },
      },
    });
    const user = userEvent.setup();
    render(<PracticeScreen />);
    await user.click(screen.getByLabelText("Square 1B"));
    await user.keyboard("q");

    await user.upload(screen.getByLabelText(/upload a picture/i), picture());

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "needs an ANTHROPIC_API_KEY",
    );
    expect(screen.getByLabelText("Square 1B")).toHaveValue("Q");
  });

  it("types downward and ignores anything that isn't a letter", async () => {
    route({});
    const user = userEvent.setup();
    render(<PracticeScreen />);
    await user.click(screen.getByRole("button", { name: "Typing: across" }));
    await user.click(screen.getByLabelText("Square 2B"));
    await user.keyboard("h7i");

    expect(screen.getByLabelText("Square 2B")).toHaveValue("H");
    expect(screen.getByLabelText("Square 3B")).toHaveValue("I");
  });
});
