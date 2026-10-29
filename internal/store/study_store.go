package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fiszki-server/internal/domain"
	"fiszki-server/internal/srs"
)

type DueCardsFilter struct {
	DeckID string
	Tag    string
	Limit  int
}

type StudyStore struct {
	pool *pgxpool.Pool
}

func NewStudyStore(pool *pgxpool.Pool) *StudyStore {
	return &StudyStore{pool: pool}
}

func (s *StudyStore) GetDueCards(ctx context.Context, userID string, filter DueCardsFilter) ([]domain.Card, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	query := `
		SELECT
			id, deck_id, user_id, front, back, tags,
			state, step, interval_days, ease_factor, repetitions, lapses,
			due_at, last_reviewed_at, created_at, updated_at
		FROM cards
		WHERE user_id = $1 AND due_at <= NOW()
	`
	args := []any{userID}
	argIdx := 2

	if deckID := strings.TrimSpace(filter.DeckID); deckID != "" {
		query += fmt.Sprintf(" AND deck_id = $%d", argIdx)
		args = append(args, deckID)
		argIdx++
	}
	if tag := strings.TrimSpace(filter.Tag); tag != "" {
		query += fmt.Sprintf(" AND $%d = ANY(tags)", argIdx)
		args = append(args, strings.ToLower(tag))
		argIdx++
	}

	// Prioritize learning/relearning first, then review, then new cards
	query += fmt.Sprintf(`
		ORDER BY
			CASE state
				WHEN 'learning' THEN 1
				WHEN 'relearning' THEN 1
				WHEN 'review' THEN 2
				WHEN 'new' THEN 3
			END ASC,
			due_at ASC
		LIMIT $%d
	`, argIdx)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query due cards: %w", err)
	}
	defer rows.Close()

	cards := make([]domain.Card, 0)
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

func (s *StudyStore) SubmitReview(ctx context.Context, userID, cardID string, rating domain.Rating) (domain.Card, domain.ReviewLog, error) {
	if !rating.IsValid() {
		return domain.Card{}, domain.ReviewLog{}, fmt.Errorf("%w: rating must be 1, 2, 3, or 4", domain.ErrValidation)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Card{}, domain.ReviewLog{}, fmt.Errorf("begin review tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx, `
		SELECT
			id, deck_id, user_id, front, back, tags,
			state, step, interval_days, ease_factor, repetitions, lapses,
			due_at, last_reviewed_at, created_at, updated_at
		FROM cards
		WHERE id = $1 AND user_id = $2
		FOR UPDATE
	`, cardID, userID)

	card, err := scanCard(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Card{}, domain.ReviewLog{}, domain.ErrNotFound
		}
		return domain.Card{}, domain.ReviewLog{}, fmt.Errorf("lock card for review: %w", err)
	}

	now := time.Now().UTC()
	next, err := srs.Schedule(card, rating, now)
	if err != nil {
		return domain.Card{}, domain.ReviewLog{}, err
	}

	updatedRow := tx.QueryRow(ctx, `
		UPDATE cards
		SET
			state = $1,
			step = $2,
			interval_days = $3,
			ease_factor = $4,
			repetitions = $5,
			lapses = $6,
			due_at = $7,
			last_reviewed_at = $8,
			updated_at = $8
		WHERE id = $9 AND user_id = $10
		RETURNING
			id, deck_id, user_id, front, back, tags,
			state, step, interval_days, ease_factor, repetitions, lapses,
			due_at, last_reviewed_at, created_at, updated_at
	`,
		next.State,
		next.Step,
		next.IntervalDays,
		next.EaseFactor,
		next.Repetitions,
		next.Lapses,
		next.DueAt,
		now,
		card.ID,
		userID,
	)

	updatedCard, err := scanCard(updatedRow)
	if err != nil {
		return domain.Card{}, domain.ReviewLog{}, fmt.Errorf("update card schedule: %w", err)
	}

	var logEntry domain.ReviewLog
	err = tx.QueryRow(ctx, `
		INSERT INTO review_logs (
			card_id, deck_id, user_id, rating,
			prev_state, new_state,
			prev_interval_days, new_interval_days,
			prev_ease_factor, new_ease_factor,
			reviewed_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING
			id, card_id, deck_id, user_id, rating,
			prev_state, new_state,
			prev_interval_days, new_interval_days,
			prev_ease_factor, new_ease_factor,
			reviewed_at
	`,
		card.ID,
		card.DeckID,
		userID,
		rating,
		card.State,
		next.State,
		card.IntervalDays,
		next.IntervalDays,
		card.EaseFactor,
		next.EaseFactor,
		now,
	).Scan(
		&logEntry.ID,
		&logEntry.CardID,
		&logEntry.DeckID,
		&logEntry.UserID,
		&logEntry.Rating,
		&logEntry.PrevState,
		&logEntry.NewState,
		&logEntry.PrevIntervalDays,
		&logEntry.NewIntervalDays,
		&logEntry.PrevEaseFactor,
		&logEntry.NewEaseFactor,
		&logEntry.ReviewedAt,
	)
	if err != nil {
		return domain.Card{}, domain.ReviewLog{}, fmt.Errorf("insert review log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Card{}, domain.ReviewLog{}, fmt.Errorf("commit review tx: %w", err)
	}

	return updatedCard, logEntry, nil
}

func (s *StudyStore) GetOverviewStats(ctx context.Context, userID string) (domain.StudyStatsOverview, error) {
	stats := domain.StudyStatsOverview{
		CardsByState: map[string]int{
			string(domain.CardStateNew):        0,
			string(domain.CardStateLearning):   0,
			string(domain.CardStateReview):     0,
			string(domain.CardStateRelearning): 0,
		},
	}

	var passedReviews int
	err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) AS total_reviews,
			COUNT(*) FILTER (WHERE reviewed_at >= date_trunc('day', NOW() AT TIME ZONE 'UTC')) AS reviews_today,
			COUNT(*) FILTER (WHERE rating >= 2) AS passed_reviews
		FROM review_logs
		WHERE user_id = $1
	`, userID).Scan(&stats.TotalReviews, &stats.ReviewsToday, &passedReviews)
	if err != nil {
		return domain.StudyStatsOverview{}, fmt.Errorf("query review stats: %w", err)
	}

	if stats.TotalReviews > 0 {
		rate := (float64(passedReviews) / float64(stats.TotalReviews)) * 100.0
		stats.RetentionRatePercent = math.Round(rate*10) / 10
	}

	rows, err := s.pool.Query(ctx, `
		SELECT state, COUNT(*)
		FROM cards
		WHERE user_id = $1
		GROUP BY state
	`, userID)
	if err != nil {
		return domain.StudyStatsOverview{}, fmt.Errorf("query cards by state: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var st string
		var count int
		if err := rows.Scan(&st, &count); err != nil {
			return domain.StudyStatsOverview{}, err
		}
		stats.CardsByState[st] = count
	}

	err = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM cards WHERE user_id = $1 AND due_at <= NOW()
	`, userID).Scan(&stats.DueNowCount)
	if err != nil {
		return domain.StudyStatsOverview{}, fmt.Errorf("query due now count: %w", err)
	}

	streak, err := s.calculateStreakDays(ctx, userID)
	if err != nil {
		return domain.StudyStatsOverview{}, err
	}
	stats.CurrentStreakDays = streak

	return stats, nil
}

func (s *StudyStore) calculateStreakDays(ctx context.Context, userID string) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT (reviewed_at AT TIME ZONE 'UTC')::date AS review_date
		FROM review_logs
		WHERE user_id = $1
		ORDER BY review_date DESC
		LIMIT 365
	`, userID)
	if err != nil {
		return 0, fmt.Errorf("query streak dates: %w", err)
	}
	defer rows.Close()

	var dates []time.Time
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return 0, err
		}
		dates = append(dates, d.UTC().Truncate(24*time.Hour))
	}
	if len(dates) == 0 {
		return 0, nil
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	yesterday := today.AddDate(0, 0, -1)

	if !dates[0].Equal(today) && !dates[0].Equal(yesterday) {
		return 0, nil
	}

	streak := 1
	expected := dates[0].AddDate(0, 0, -1)
	for i := 1; i < len(dates); i++ {
		if dates[i].Equal(expected) {
			streak++
			expected = expected.AddDate(0, 0, -1)
		} else {
			break
		}
	}
	return streak, nil
}
