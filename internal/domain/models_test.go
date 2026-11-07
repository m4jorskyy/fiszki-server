package domain

import "testing"

func TestRating_IsValid(t *testing.T) {
	valid := []Rating{RatingAgain, RatingHard, RatingGood, RatingEasy}
	for _, r := range valid {
		if !r.IsValid() {
			t.Fatalf("expected rating %d to be valid", r)
		}
	}

	invalid := []Rating{-1, 0, 5, 100}
	for _, r := range invalid {
		if r.IsValid() {
			t.Fatalf("expected rating %d to be invalid", r)
		}
	}
}
