package session

import (
	"rackwise/engine/internal/bot"
	"rackwise/engine/internal/game"
)

const (
	personName = "you"
	botName    = "bot"
)

// Scores gives each side's points.
type Scores struct {
	You int `json:"you"`
	Bot int `json:"bot"`
}

// View is what the person may see of a game. It is built field by field from the
// record, and has no field for the bot's rack or the bag until the game is over,
// so hidden information can't reach the browser by accident.
type View struct {
	ID           string     `json:"id"`
	Level        bot.Level  `json:"level"`
	Status       string     `json:"status"`
	Board        []string   `json:"board"`
	Rack         string     `json:"rack"`
	Scores       Scores     `json:"scores"`
	BagRemaining int        `json:"bag_remaining"`
	BotTiles     int        `json:"bot_tiles"`
	CanExchange  bool       `json:"can_exchange"`
	Turns        []TurnView `json:"turns"`
	Result       *Result    `json:"result,omitempty"`
}

// TurnView is one line in the move list. A bot's exchange shows only a count:
// its rack before the turn and the tiles it gave up stay hidden.
type TurnView struct {
	Player         string           `json:"player"`
	Kind           string           `json:"kind"`
	Position       string           `json:"position,omitempty"`
	Word           string           `json:"word,omitempty"`
	CrossWords     []string         `json:"cross_words,omitempty"`
	Tiles          []game.Placement `json:"tiles,omitempty"`
	Bingo          bool             `json:"bingo,omitempty"`
	Exchanged      string           `json:"exchanged,omitempty"`
	ExchangedCount int              `json:"exchanged_count,omitempty"`
	Score          int              `json:"score"`
	Total          int              `json:"total"`
}

// Result describes a finished game. Racks are revealed now that it's over.
type Result struct {
	Outcome     string `json:"outcome"` // win, loss or draw, from the person's side
	Reason      string `json:"reason"`  // went_out or scoreless
	YourRack    string `json:"your_rack"`
	BotRack     string `json:"bot_rack"`
	Adjustments Scores `json:"adjustments"`
}

func newView(record *Record) *View {
	state := &record.State
	view := &View{
		ID:           record.ID,
		Level:        record.Level,
		Status:       "active",
		Board:        state.Board,
		Rack:         state.Racks[personSeat],
		Scores:       Scores{You: state.Scores[personSeat], Bot: state.Scores[botSeat]},
		BagRemaining: len(state.Bag),
		BotTiles:     len(state.Racks[botSeat]),
		CanExchange:  !state.Over && len(state.Bag) >= game.MinBagForExchange,
		Turns:        make([]TurnView, 0, len(state.Turns)),
	}

	totals := [2]int{}
	for _, turn := range state.Turns {
		totals[turn.Player] += turn.Score
		line := TurnView{
			Player: seatName(turn.Player),
			Kind:   string(turn.Kind),
			Score:  turn.Score,
			Total:  totals[turn.Player],
		}
		switch turn.Kind {
		case game.TurnPlay:
			line.Position = turn.Move.Position()
			line.Word = turn.Move.Word
			line.CrossWords = turn.Move.CrossWords
			line.Tiles = turn.Move.Tiles
			line.Bingo = turn.Move.Bingo
		case game.TurnExchange:
			line.ExchangedCount = len(turn.Exchanged)
			if turn.Player == personSeat {
				line.Exchanged = turn.Exchanged
			}
		}
		view.Turns = append(view.Turns, line)
	}

	if state.Over {
		view.Status = "finished"
		view.Result = &Result{
			Outcome:     outcome(state.Scores),
			Reason:      "scoreless",
			YourRack:    state.Racks[personSeat],
			BotRack:     state.Racks[botSeat],
			Adjustments: Scores{You: state.EndAdjustments[personSeat], Bot: state.EndAdjustments[botSeat]},
		}
		if state.Racks[personSeat] == "" || state.Racks[botSeat] == "" {
			view.Result.Reason = "went_out"
		}
	}
	return view
}

func seatName(seat int) string {
	if seat == personSeat {
		return personName
	}
	return botName
}

func outcome(scores [2]int) string {
	switch {
	case scores[personSeat] > scores[botSeat]:
		return "win"
	case scores[personSeat] < scores[botSeat]:
		return "loss"
	}
	return "draw"
}
