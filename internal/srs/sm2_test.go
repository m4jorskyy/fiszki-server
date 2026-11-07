package srs

import (
	"errors"
	"testing"
	"time"

	"fiszki-server/internal/domain"
)

func TestSchedule_NewToReviewLifecycle(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	card := domain.Card{
		State:      domain.CardStateNew,
		EaseFactor: DefaultEaseFactor,
	}

	// Step 1: Rate Good on a new card -> moves to learning step 1 (+10m)
	res1, err := Schedule(card, domain.RatingGood, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res1.State != domain.CardStateLearning || res1.Step != 1 {
		t.Fatalf("expected learning step 1, got state=%s step=%d", res1.State, res1.Step)
	}
	if !res1.DueAt.Equal(now.Add(10 * time.Minute)) {
		t.Fatalf("expected due +10m, got %v", res1.DueAt)
	}

	// Step 2: Rate Good on learning step 1 -> graduates to review (1 day)
	card.State = res1.State
	card.Step = res1.Step
	res2, err := Schedule(card, domain.RatingGood, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res2.State != domain.CardStateReview || res2.IntervalDays != 1 || res2.Repetitions != 1 {
		t.Fatalf("expected review interval=1 reps=1, got %+v", res2)
	}

	// Step 3: Rate Good on first review -> interval becomes 6 days
	card.State = res2.State
	card.IntervalDays = res2.IntervalDays
	card.Repetitions = res2.Repetitions
	res3, err := Schedule(card, domain.RatingGood, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res3.IntervalDays != 6 || res3.Repetitions != 2 {
		t.Fatalf("expected interval=6 reps=2, got %+v", res3)
	}

	// Step 4: Rate Good on second review -> interval = round(6 * 2.5) = 15 days
	card.IntervalDays = res3.IntervalDays
	card.Repetitions = res3.Repetitions
	res4, err := Schedule(card, domain.RatingGood, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res4.IntervalDays != 15 || res4.Repetitions != 3 {
		t.Fatalf("expected interval=15 reps=3, got %+v", res4)
	}

	// Step 5: Rate Again on review card -> lapse into relearning, ease drops to 2.3
	card.IntervalDays = res4.IntervalDays
	card.Repetitions = res4.Repetitions
	res5, err := Schedule(card, domain.RatingAgain, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res5.State != domain.CardStateRelearning || res5.Lapses != 1 || res5.EaseFactor != 2.3 {
		t.Fatalf("expected relearning lapses=1 ef=2.3, got %+v", res5)
	}

	// Step 6: Rate Good on relearning -> graduates back to review (1 day)
	card.State = res5.State
	card.Lapses = res5.Lapses
	card.EaseFactor = res5.EaseFactor
	card.IntervalDays = res5.IntervalDays
	res6, err := Schedule(card, domain.RatingGood, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res6.State != domain.CardStateReview || res6.IntervalDays != 1 {
		t.Fatalf("expected review interval=1, got %+v", res6)
	}
}

func TestSchedule_AllStateAndRatingBranches(t *testing.T) {
	now := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)

	t.Run("invalid rating returns ErrValidation", func(t *testing.T) {
		_, err := Schedule(domain.Card{State: domain.CardStateNew}, domain.Rating(0), now)
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
		_, err = Schedule(domain.Card{State: domain.CardStateNew}, domain.Rating(5), now)
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("new card Again, Hard, Easy", func(t *testing.T) {
		card := domain.Card{State: domain.CardStateNew, Step: 1, EaseFactor: 0} // ease < MinEaseFactor defaults to 2.5

		again, err := Schedule(card, domain.RatingAgain, now)
		if err != nil || again.State != domain.CardStateLearning || again.Step != 0 || !again.DueAt.Equal(now.Add(1*time.Minute)) || again.EaseFactor != DefaultEaseFactor {
			t.Fatalf("unexpected new->Again result: %+v, err=%v", again, err)
		}

		hard, err := Schedule(card, domain.RatingHard, now)
		if err != nil || hard.State != domain.CardStateLearning || !hard.DueAt.Equal(now.Add(6*time.Minute)) {
			t.Fatalf("unexpected new->Hard result: %+v, err=%v", hard, err)
		}

		easy, err := Schedule(card, domain.RatingEasy, now)
		if err != nil || easy.State != domain.CardStateReview || easy.IntervalDays != 4 || easy.Repetitions != 1 || !easy.DueAt.Equal(now.Add(4*24*time.Hour)) {
			t.Fatalf("unexpected new->Easy result: %+v, err=%v", easy, err)
		}
	})

	t.Run("review card Hard and Easy", func(t *testing.T) {
		card := domain.Card{
			State:        domain.CardStateReview,
			IntervalDays: 10,
			EaseFactor:   2.5,
			Repetitions:  2,
		}

		hard, err := Schedule(card, domain.RatingHard, now)
		if err != nil || hard.IntervalDays != 12 || hard.EaseFactor != 2.35 || hard.Repetitions != 3 {
			t.Fatalf("unexpected review->Hard result: %+v, err=%v", hard, err)
		}

		easy, err := Schedule(card, domain.RatingEasy, now)
		// 10 * 2.5 * 1.3 = 32.5 -> round to 33
		if err != nil || easy.IntervalDays != 33 || easy.EaseFactor != 2.65 || easy.Repetitions != 3 {
			t.Fatalf("unexpected review->Easy result: %+v, err=%v", easy, err)
		}
	})

	t.Run("relearning card Again, Hard, Easy", func(t *testing.T) {
		card := domain.Card{
			State:        domain.CardStateRelearning,
			IntervalDays: 3,
			EaseFactor:   2.1,
		}

		again, err := Schedule(card, domain.RatingAgain, now)
		if err != nil || again.State != domain.CardStateRelearning || !again.DueAt.Equal(now.Add(1*time.Minute)) {
			t.Fatalf("unexpected relearning->Again result: %+v, err=%v", again, err)
		}

		hard, err := Schedule(card, domain.RatingHard, now)
		if err != nil || hard.State != domain.CardStateRelearning || !hard.DueAt.Equal(now.Add(6*time.Minute)) {
			t.Fatalf("unexpected relearning->Hard result: %+v, err=%v", hard, err)
		}

		easy, err := Schedule(card, domain.RatingEasy, now)
		if err != nil || easy.State != domain.CardStateReview || easy.IntervalDays != 4 || easy.Repetitions != 1 {
			t.Fatalf("unexpected relearning->Easy result: %+v, err=%v", easy, err)
		}
	})
}

func TestSchedule_EaseFactorFloor(t *testing.T) {
	now := time.Now().UTC()
	card := domain.Card{
		State:        domain.CardStateReview,
		IntervalDays: 10,
		EaseFactor:   1.35,
		Repetitions:  3,
	}

	res, err := Schedule(card, domain.RatingAgain, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.EaseFactor != MinEaseFactor {
		t.Fatalf("expected ease factor floor %v, got %v", MinEaseFactor, res.EaseFactor)
	}
}
