# 0005: Leave values learned from self-play

## Context

The hard bot and the coach need to estimate what the tiles kept after a play are worth. Rackwise has its own tile set and dictionary (`0003-original-rules.md`), so published tables don't apply and the values have to be learned.

## Decision

A leave's value is the sum of a value per tile and an adjustment per pair of tiles. `cmd/genleaves` learns them from self-play and writes `engine/data/leaves.json`.

1. Play 20,000 games between highest-score bots, recording the tiles kept each turn and the same player's score next turn. Turns with fewer than 7 tiles left in the bag are skipped.
2. Fit `next score = baseline + tile values + pair adjustments` with ridge regression. Pairs are shrunk harder, so rarely seen pairs stay near zero. The baseline is dropped, so values mean "compared with keeping nothing and drawing a full rack".
3. Repeat with 20,000 games between bots using the first values, so the data resembles how the hard bot plays.

The result behaves as expected. A blank is worth +14.3, a Q costs 4.9, and QU earns back 7.2. Duplicates are penalised, with SS at -6.1 and UU at -5.7.

Regression was chosen over averaging each tile's results separately, because a leave's value is a sum. Separate averages would count the same effect several times when added up.

## Bot strength

Measured with `cmd/selfplay` on seeds not used for tuning, with each side going first in half the games. Ranges are 95% confidence.

| Match | Games | Win rate | Mean spread |
|---|---|---|---|
| Hard vs medium | 6,000 | 60.0% +/- 1.2 | +23.2 +/- 2.3 |
| Medium vs easy | 2,000 | 99.9% +/- 0.2 | +242.9 +/- 3.9 |
| Hard vs easy | 2,000 | 99.8% +/- 0.2 | +261.9 +/- 3.8 |

## The lane penalty and exchange threshold have no measured benefit

The specification asks the hard bot to penalise opening triple-word lanes, so it does, at 3 points per lane. Against medium, a sweep of 4,000 games each gave spreads of +19.2 at no penalty, +23.4 at 3 and +15.8 at 8, so a large penalty hurts. The small gain at 3 is within the noise and was picked from a sweep, so it may be flattering. In a direct match on fresh seeds, a hard bot without the penalty scored level with one that had it: 49.6% +/- 1.3 for the side without.

The exchange threshold is similar. Thresholds from 0 to 10 are indistinguishable, and 20 is clearly worse. It is set to 5. A direct match with a threshold of 0 gave 49.9% +/- 1.3.

So the hard bot's advantage comes from leave values. `lanes_opened` is still reported on every move, because the coach can describe an opening whether or not a fixed penalty is the right way to price it.

## Trade-offs

- It only looks one turn ahead, so it misses longer plans and the worth of tiles in the endgame.
- It is noisier and cruder than a full table of every possible leave, which would have about 900,000 entries, but has 405 numbers that can be inspected and explained.
- Values reflect bot play against bots. They may not match human opponents.
- Results depend on the rules and dictionary, so changing either means rerunning `cmd/genleaves`.
