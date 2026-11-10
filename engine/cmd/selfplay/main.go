// Command selfplay plays bots against each other and reports the result.
//
//	go run ./cmd/selfplay -a hard -b medium -games 2000
package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"runtime"
	"sync"

	"rackwise/engine/internal/bot"
	"rackwise/engine/internal/eval"
)

type result struct {
	wins, draws, turns       int
	scoreA, scoreB, spreadSq float64
}

func main() {
	nameA := flag.String("a", "hard", "level of bot A")
	nameB := flag.String("b", "medium", "level of bot B")
	games := flag.Int("games", 1000, "games to play")
	seed := flag.Uint64("seed", 1, "first game's seed")
	lanePenalty := flag.Float64("lane-penalty", eval.DefaultLanePenalty, "bot A's penalty per lane opened")
	exchangeBelow := flag.Float64("exchange-below", bot.DefaultExchangeBelow, "bot A exchanges below this equity")
	flag.Parse()

	levelA, err := bot.ParseLevel(*nameA)
	if err != nil {
		log.Fatal(err)
	}
	levelB, err := bot.ParseLevel(*nameB)
	if err != nil {
		log.Fatal(err)
	}
	base, err := bot.Default()
	if err != nil {
		log.Fatal(err)
	}
	tuned := *base
	tuned.Eval = &eval.Evaluator{Leaves: base.Eval.Leaves, LanePenalty: *lanePenalty}
	tuned.ExchangeBelow = *exchangeBelow
	a := bot.Agent{Bots: &tuned, Level: levelA}
	b := bot.Agent{Bots: base, Level: levelB}

	workers := runtime.GOMAXPROCS(0)
	results := make([]result, workers)
	var wg sync.WaitGroup
	for worker := range workers {
		wg.Go(func() {
			r := &results[worker]
			for i := worker; i < *games; i += workers {
				// Swap who goes first each game, since moving first is an advantage.
				aSeat := i % 2
				agents := [2]bot.Agent{a, b}
				if aSeat == 1 {
					agents = [2]bot.Agent{b, a}
				}
				g, err := bot.PlayGame(agents, *seed+uint64(i))
				if err != nil {
					log.Fatalf("game %d: %v", i, err)
				}
				scoreA, scoreB := float64(g.Scores[aSeat]), float64(g.Scores[1-aSeat])
				switch {
				case scoreA > scoreB:
					r.wins++
				case scoreA == scoreB:
					r.draws++
				}
				r.scoreA += scoreA
				r.scoreB += scoreB
				r.spreadSq += (scoreA - scoreB) * (scoreA - scoreB)
				r.turns += len(g.Turns)
			}
		})
	}
	wg.Wait()

	var total result
	for _, r := range results {
		total.wins += r.wins
		total.draws += r.draws
		total.turns += r.turns
		total.scoreA += r.scoreA
		total.scoreB += r.scoreB
		total.spreadSq += r.spreadSq
	}
	n := float64(*games)
	winRate := (float64(total.wins) + float64(total.draws)/2) / n
	meanSpread := (total.scoreA - total.scoreB) / n
	spreadError := math.Sqrt(max(0, total.spreadSq/n-meanSpread*meanSpread) / n)
	fmt.Printf("%s (A) vs %s (B), %d games, lane penalty %.1f, exchange below %.1f\n",
		levelA, levelB, *games, *lanePenalty, *exchangeBelow)
	fmt.Printf("A win rate %.1f%% +/- %.1f (draws %d)\n", winRate*100, 196*math.Sqrt(winRate*(1-winRate)/n), total.draws)
	fmt.Printf("mean score A %.1f, B %.1f, spread %+.1f +/- %.1f, %.1f turns per game\n",
		total.scoreA/n, total.scoreB/n, meanSpread, 1.96*spreadError, float64(total.turns)/n)
}
