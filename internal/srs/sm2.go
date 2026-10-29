package srs

import (
	"fmt"
	"math"
	"time"

	"fiszki-server/internal/domain"
)

const (
	DefaultEaseFactor = 2.5
	MinEaseFactor     = 1.3
)

type ScheduleResult struct {
	State        domain.CardState
	Step         int
	IntervalDays int
	EaseFactor   float64
	Repetitions  int
	Lapses       int
	DueAt        time.Time
}

// Schedule calculates the next SM-2 scheduling state for a card given a rating (1..4) and review timestamp.
func Schedule(card domain.Card, rating domain.Rating, now time.Time) (ScheduleResult, error) {
	if !rating.IsValid() {
		return ScheduleResult{}, fmt.Errorf("%w: rating must be between 1 and 4", domain.ErrValidation)
	}

	ef := card.EaseFactor
	if ef < MinEaseFactor {
		ef = DefaultEaseFactor
	}

	res := ScheduleResult{
		State:        card.State,
		Step:         card.Step,
		IntervalDays: card.IntervalDays,
		EaseFactor:   ef,
		Repetitions:  card.Repetitions,
		Lapses:       card.Lapses,
	}

	switch card.State {
	case domain.CardStateNew, domain.CardStateLearning, "":
		switch rating {
		case domain.RatingAgain:
			res.State = domain.CardStateLearning
			res.Step = 0
			res.DueAt = now.Add(1 * time.Minute)
		case domain.RatingHard:
			res.State = domain.CardStateLearning
			res.DueAt = now.Add(6 * time.Minute)
		case domain.RatingGood:
			if card.Step == 0 {
				res.State = domain.CardStateLearning
				res.Step = 1
				res.DueAt = now.Add(10 * time.Minute)
			} else {
				res.State = domain.CardStateReview
				res.Step = 0
				res.Repetitions = 1
				res.IntervalDays = 1
				res.DueAt = now.Add(24 * time.Hour)
			}
		case domain.RatingEasy:
			res.State = domain.CardStateReview
			res.Step = 0
			res.Repetitions = 1
			res.IntervalDays = 4
			res.DueAt = now.Add(4 * 24 * time.Hour)
		}

	case domain.CardStateReview:
		curInterval := maxInt(1, card.IntervalDays)
		switch rating {
		case domain.RatingAgain:
			res.State = domain.CardStateRelearning
			res.Step = 0
			res.Lapses = card.Lapses + 1
			res.Repetitions = 0
			res.EaseFactor = math.Max(MinEaseFactor, ef-0.20)
			res.IntervalDays = 1
			res.DueAt = now.Add(10 * time.Minute)
		case domain.RatingHard:
			res.State = domain.CardStateReview
			res.EaseFactor = math.Max(MinEaseFactor, ef-0.15)
			nextInterval := int(math.Round(float64(curInterval) * 1.2))
			res.IntervalDays = maxInt(curInterval+1, nextInterval)
			res.Repetitions = card.Repetitions + 1
			res.DueAt = now.Add(time.Duration(res.IntervalDays) * 24 * time.Hour)
		case domain.RatingGood:
			res.State = domain.CardStateReview
			if card.Repetitions <= 1 {
				res.IntervalDays = maxInt(curInterval+1, 6)
			} else {
				nextInterval := int(math.Round(float64(curInterval) * ef))
				res.IntervalDays = maxInt(curInterval+1, nextInterval)
			}
			res.Repetitions = card.Repetitions + 1
			res.DueAt = now.Add(time.Duration(res.IntervalDays) * 24 * time.Hour)
		case domain.RatingEasy:
			res.State = domain.CardStateReview
			res.EaseFactor = ef + 0.15
			nextInterval := int(math.Round(float64(curInterval) * ef * 1.3))
			res.IntervalDays = maxInt(curInterval+1, nextInterval)
			res.Repetitions = card.Repetitions + 1
			res.DueAt = now.Add(time.Duration(res.IntervalDays) * 24 * time.Hour)
		}

	case domain.CardStateRelearning:
		switch rating {
		case domain.RatingAgain:
			res.State = domain.CardStateRelearning
			res.Step = 0
			res.DueAt = now.Add(1 * time.Minute)
		case domain.RatingHard:
			res.State = domain.CardStateRelearning
			res.DueAt = now.Add(6 * time.Minute)
		case domain.RatingGood:
			res.State = domain.CardStateReview
			res.Step = 0
			res.Repetitions = 1
			res.IntervalDays = maxInt(1, card.IntervalDays)
			res.DueAt = now.Add(time.Duration(res.IntervalDays) * 24 * time.Hour)
		case domain.RatingEasy:
			res.State = domain.CardStateReview
			res.Step = 0
			res.Repetitions = 1
			res.IntervalDays = maxInt(2, card.IntervalDays+1)
			res.DueAt = now.Add(time.Duration(res.IntervalDays) * 24 * time.Hour)
		}
	}

	res.EaseFactor = math.Round(res.EaseFactor*100) / 100
	return res, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
