// Command genleaves learns leave values from self-play and writes data/leaves.json.
//
//	go run ./cmd/genleaves -games 20000 -out data/leaves.json
//
// For each turn, it records the tiles a player kept and what they scored on their
// next turn, then fits a linear model: next score = baseline + the value of each
// kept tile + an adjustment for each kept pair. A tile's learned value therefore
// says how much keeping it helps next turn, compared with drawing a fresh tile.
//
// It runs two rounds. The first uses highest-score bots, which ignore leaves. The
// second uses bots playing by the first round's values, so the data resembles how
// the hard bot really plays. See docs/decisions/0005-leave-values.md.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"slices"
	"sync"

	"rackwise/engine/internal/bot"
	"rackwise/engine/internal/eval"
	"rackwise/engine/internal/game"
)

const (
	tileKinds = 27
	// One feature for the baseline, then singles, then pairs.
	pairFeatures = tileKinds * (tileKinds + 1) / 2
	features     = 1 + tileKinds + pairFeatures

	// Ridge strengths. Pairs get strong shrinkage because rare pairs are seen
	// too seldom to trust; with no evidence they stay near zero.
	singleRidge = 1.0
	pairRidge   = 100.0

	// minBagAfter skips turns near the end, when the next rack depends on what's
	// left rather than on what was kept.
	minBagAfter = game.RackSize
)

func main() {
	games := flag.Int("games", 20000, "games per round")
	seed := flag.Uint64("seed", 1, "first game's seed")
	out := flag.String("out", "data/leaves.json", "output file")
	flag.Parse()

	base, err := bot.Default()
	if err != nil {
		log.Fatal(err)
	}

	first := fit(bot.Agent{Bots: base, Level: bot.Medium}, *games, *seed)
	leaves, err := first.Leaves()
	if err != nil {
		log.Fatal(err)
	}

	informed := *base
	informed.Eval = &eval.Evaluator{Leaves: leaves}
	second := fit(bot.Agent{Bots: &informed, Level: bot.Hard}, *games, *seed+uint64(*games))

	text, err := json.MarshalIndent(second, "", " ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, append(text, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	report(second)
}

// normal accumulates the normal equations for least squares: A x = b.
type normal struct {
	a       []float64 // features x features
	b       [features]float64
	samples int
}

func (n *normal) add(active []int, y float64) {
	for _, i := range active {
		n.b[i] += y
		for _, j := range active {
			n.a[i*features+j]++
		}
	}
	n.samples++
}

func pairIndex(i, j int) int {
	if i > j {
		i, j = j, i
	}
	// Pairs (i, j) with i <= j, laid out row by row.
	return 1 + tileKinds + i*tileKinds - i*(i-1)/2 + (j - i)
}

// activeFeatures lists a leave's features. A feature can repeat, as with a
// doubled tile, which is how its count enters the model.
func activeFeatures(leave string, into []int) []int {
	into = append(into[:0], 0)
	for i := range len(leave) {
		a := eval.TileIndex(leave[i])
		into = append(into, 1+a)
		for j := i + 1; j < len(leave); j++ {
			into = append(into, pairIndex(a, eval.TileIndex(leave[j])))
		}
	}
	return into
}

func fit(agent bot.Agent, games int, seed uint64) *eval.LeavesFile {
	workers := runtime.GOMAXPROCS(0)
	parts := make([]*normal, workers)
	var wg sync.WaitGroup
	for worker := range workers {
		parts[worker] = &normal{a: make([]float64, features*features)}
		wg.Go(func() {
			var active []int
			for i := worker; i < games; i += workers {
				g, err := bot.PlayGame([2]bot.Agent{agent, agent}, seed+uint64(i))
				if err != nil {
					log.Fatalf("game %d: %v", i, err)
				}
				active = collect(g, parts[worker], active)
			}
		})
	}
	wg.Wait()

	total := &normal{a: make([]float64, features*features)}
	for _, part := range parts {
		for i := range part.a {
			total.a[i] += part.a[i]
		}
		for i := range part.b {
			total.b[i] += part.b[i]
		}
		total.samples += part.samples
	}
	log.Printf("fitting %d samples from %d %s games", total.samples, games, agent.Level)
	return solve(total)
}

// collect pairs each turn's leave with the same player's next score.
func collect(g *game.Game, n *normal, active []int) []int {
	for i, turn := range g.Turns {
		if turn.BagAfter < minBagAfter || i+2 >= len(g.Turns) {
			continue
		}
		leave, err := keptTiles(turn)
		if err != nil {
			log.Fatal(err)
		}
		active = activeFeatures(leave, active)
		n.add(active, float64(g.Turns[i+2].Score))
	}
	return active
}

func keptTiles(turn game.Turn) (string, error) {
	switch turn.Kind {
	case game.TurnPlay:
		return game.RemoveTiles(turn.Rack, turn.Move.Tiles)
	case game.TurnExchange:
		return game.RemoveExchanged(turn.Rack, turn.Exchanged)
	}
	return turn.Rack, nil
}

// solve runs ridge regression with a Cholesky factorization. The baseline isn't
// shrunk or stored: values are relative to keeping nothing.
func solve(n *normal) *eval.LeavesFile {
	a := slices.Clone(n.a)
	for i := 1; i < features; i++ {
		ridge := singleRidge
		if i > tileKinds {
			ridge = pairRidge
		}
		a[i*features+i] += ridge
	}
	// A tiny term keeps features never seen (like a pair of Qs) solvable.
	for i := range features {
		a[i*features+i] += 1e-9
	}

	for i := range features {
		for j := 0; j <= i; j++ {
			sum := a[i*features+j]
			for k := range j {
				sum -= a[i*features+k] * a[j*features+k]
			}
			if i == j {
				a[i*features+i] = math.Sqrt(sum)
			} else {
				a[i*features+j] = sum / a[j*features+j]
			}
		}
	}
	var x [features]float64
	for i := range features {
		sum := n.b[i]
		for k := range i {
			sum -= a[i*features+k] * x[k]
		}
		x[i] = sum / a[i*features+i]
	}
	for i := features - 1; i >= 0; i-- {
		sum := x[i]
		for k := i + 1; k < features; k++ {
			sum -= a[k*features+i] * x[k]
		}
		x[i] = sum / a[i*features+i]
	}

	file := &eval.LeavesFile{Singles: map[string]float64{}, Pairs: map[string]float64{}}
	for i := range tileKinds {
		file.Singles[tileName(i)] = round(x[1+i])
		for j := i; j < tileKinds; j++ {
			if value := round(x[pairIndex(i, j)]); value != 0 {
				file.Pairs[pairName(i, j)] = value
			}
		}
	}
	return file
}

func tileName(i int) string {
	if i == tileKinds-1 {
		return string(rune(game.Blank))
	}
	return string(rune('A' + i))
}

// pairName puts the blank first, matching sorted rack order.
func pairName(i, j int) string {
	if j == tileKinds-1 {
		return tileName(j) + tileName(i)
	}
	return tileName(i) + tileName(j)
}

func round(value float64) float64 {
	return math.Round(value*100) / 100
}

func report(file *eval.LeavesFile) {
	type entry struct {
		name  string
		value float64
	}
	var singles []entry
	for name, value := range file.Singles {
		singles = append(singles, entry{name, value})
	}
	slices.SortFunc(singles, func(a, b entry) int { return int(math.Round((b.value - a.value) * 100)) })
	for _, s := range singles {
		fmt.Printf("%s %+.2f\n", s.name, s.value)
	}
	for _, pair := range []string{"QU", "EE", "II", "UU", "VV", "SS", "??"} {
		fmt.Printf("%s %+.2f\n", pair, file.Pairs[pair])
	}
}
