package domain

import "time"

type CardState string

const (
	CardStateNew        CardState = "new"
	CardStateLearning   CardState = "learning"
	CardStateReview     CardState = "review"
	CardStateRelearning CardState = "relearning"
)

type Rating int

const (
	RatingAgain Rating = 1
	RatingHard  Rating = 2
	RatingGood  Rating = 3
	RatingEasy  Rating = 4
)

func (r Rating) IsValid() bool {
	return r >= RatingAgain && r <= RatingEasy
}

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Deck struct {
	ID          string      `json:"id"`
	UserID      string      `json:"user_id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Counts      DeckCounts  `json:"counts"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

type DeckCounts struct {
	TotalCards    int `json:"total_cards"`
	NewCards      int `json:"new_cards"`
	LearningCards int `json:"learning_cards"`
	DueCards      int `json:"due_cards"`
}

type Card struct {
	ID             string     `json:"id"`
	DeckID         string     `json:"deck_id"`
	UserID         string     `json:"user_id"`
	Front          string     `json:"front"`
	Back           string     `json:"back"`
	Tags           []string   `json:"tags"`
	State          CardState  `json:"state"`
	Step           int        `json:"step"`
	IntervalDays   int        `json:"interval_days"`
	EaseFactor     float64    `json:"ease_factor"`
	Repetitions    int        `json:"repetitions"`
	Lapses         int        `json:"lapses"`
	DueAt          time.Time  `json:"due_at"`
	LastReviewedAt *time.Time `json:"last_reviewed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type CardInput struct {
	Front string   `json:"front"`
	Back  string   `json:"back"`
	Tags  []string `json:"tags,omitempty"`
}

type ExtractedCardCandidate struct {
	Front string `json:"front"`
	Back  string `json:"back"`
}

type ReviewLog struct {
	ID               string    `json:"id"`
	CardID           string    `json:"card_id"`
	DeckID           string    `json:"deck_id"`
	UserID           string    `json:"user_id"`
	Rating           Rating    `json:"rating"`
	PrevState        CardState `json:"prev_state"`
	NewState         CardState `json:"new_state"`
	PrevIntervalDays int       `json:"prev_interval_days"`
	NewIntervalDays  int       `json:"new_interval_days"`
	PrevEaseFactor   float64   `json:"prev_ease_factor"`
	NewEaseFactor    float64   `json:"new_ease_factor"`
	ReviewedAt       time.Time `json:"reviewed_at"`
}

type StudyStatsOverview struct {
	ReviewsToday         int            `json:"reviews_today"`
	TotalReviews         int            `json:"total_reviews"`
	CurrentStreakDays    int            `json:"current_streak_days"`
	RetentionRatePercent float64        `json:"retention_rate_percent"`
	CardsByState         map[string]int `json:"cards_by_state"`
	DueNowCount          int            `json:"due_now_count"`
}
