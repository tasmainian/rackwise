"""MCP server for development lookups. Phase 7 adds the coach's tools here."""

from mcp.server.fastmcp import FastMCP

from rackwise_coach import word_lookup

server = FastMCP("rackwise")


@server.tool()
def lookup_words(rack: str, board_letters: str = "") -> dict:
    """List NWL23 words that can be made from a rack and board letters, using word.tips.

    Args:
        rack: Your tiles, 1 to 7 of A-Z, with ? for a blank.
        board_letters: Up to 15 board letters you could play through, such as those in
            one row or column.

    word.tips is the only source of points here: report `site_points` exactly as returned and
    never calculate or adjust them. They cover the word's letters alone, so a play's final
    score on a board would also depend on premium squares, cross-words and bonuses. A listed
    word might not fit on the board, and Rackwise's engine might reject it.
    `blank_positions` holds zero-based letter positions filled by a blank. Each word length
    shows at most 30 words, and `more_available` says when some were cut off.
    """
    return word_lookup.lookup_words(rack, board_letters)


def main() -> None:
    server.run()


if __name__ == "__main__":
    main()
