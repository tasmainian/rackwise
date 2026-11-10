# 0009: Fill in the board from a picture

## Context

Typing up to 100 tiles onto the grid is slow and easy to get wrong. The developer asked to upload a picture of their board instead.

## Decision

The player uploads a screenshot, and Claude's vision returns the tiles as 15 rows of 15 characters, plus the rack. The grid fills in straight away, the screen asks the player to check it, and **Undo upload** restores what was there before. Any square can be corrected by typing.

Claude only transcribes here. Word validity and scoring stay with word.tips and `board.py`, so this doesn't change the rule that the LLM never decides those.

## Why

- **A vision model over classic image processing.** Finding a grid and recognising letters by hand would be a project of its own, and would break on different phones, cropping or themes. One API call handles those and fits the project's stack.
- **The multiplier layout is sent as landmarks.** Being one square out spoils a review, and the visible labels help place tiles.
- **Higher effort than coaching.** The reading gets `high` effort, because the rest of the review relies on it.
- **Code checks the shape of the answer.** Rows must be exactly 15 by 15 of `A-Z` or `.`, and the rack must be up to 7 of `A-Z` or `?`. The image's type comes from its bytes. Because the output is that constrained, text inside a picture has little room to do harm.
- **Sent as base64 in JSON,** which avoids a multipart dependency for one small upload. Base64 doesn't affect cost: Claude charges for an image by its pixel size.
- **Shrunk in the browser first.** The picture is redrawn at most 1568 pixels on its long edge and sent as JPEG at quality 0.9. Image tokens grow with pixel count, and Sonnet 5.5 accepts up to 2576 pixels. Measured with the token counter on Sonnet 5.5: a full iPhone screenshot (1290×2796) is 3,959 image tokens, and shrunk to 723×1568 it's 1,459. That cuts a read's input from 4,594 to 2,094 tokens. This also keeps large photos under the 5 MB limit. If the browser can't draw the picture, the original is sent.
- **Token use is logged** for every Claude call, so real costs can be measured.

## Trade-offs

- **Accuracy is unmeasured.** It hasn't been run on a real picture, because no API key was available when it was written. Misplaced or misread tiles are possible, so the player must check the grid. A small set of real screenshots with known boards would measure this.
- **It needs an API key and costs money per upload,** with no fallback other than typing.
- **Pictures go to the Claude API.** A game screenshot can include things like a username or notifications.
- **Blanks on the board stay unmarked**, so they're read as normal letters and those scores can be too high.
- **Shrinking could cost accuracy** on small tiles. This hasn't been measured on a real screenshot yet.
- **The 5 MB limit still applies** when the browser can't shrink a picture.
