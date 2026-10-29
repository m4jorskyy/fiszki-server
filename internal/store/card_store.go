package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fiszki-server/internal/domain"
	"fiszki-server/internal/srs"
)

type CardFilter struct {
	Tag   string
	State string
	Query string
}

type CardStore struct {
	pool *pgxpool.Pool
}

func NewCardStore(pool *pgxpool.Pool) *CardStore {
	return &CardStore{pool: pool}
}

func (s *CardStore) verifyDeckOwnership(ctx context.Context, tx pgx.Tx, userID, deckID string) error {
	var exists bool
	var err error
	if tx != nil {
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM decks WHERE id = $1 AND user_id = $2)`, deckID, userID).Scan(&exists)
	} else {
		err = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM decks WHERE id = $1 AND user_id = $2)`, deckID, userID).Scan(&exists)
	}
	if err != nil {
		return fmt.Errorf("check deck ownership: %w", err)
	}
	if !exists {
		return domain.ErrNotFound
	}
	return nil
}

func (s *CardStore) ListCards(ctx context.Context, userID, deckID string, filter CardFilter) ([]domain.Card, error) {
	if err := s.verifyDeckOwnership(ctx, nil, userID, deckID); err != nil {
		return nil, err
	}

	query := `
		SELECT
			id, deck_id, user_id, front, back, tags,
			state, step, interval_days, ease_factor, repetitions, lapses,
			due_at, last_reviewed_at, created_at, updated_at
		FROM cards
		WHERE deck_id = $1 AND user_id = $2
	`
	args := []any{deckID, userID}
	argIdx := 3

	if tag := strings.TrimSpace(filter.Tag); tag != "" {
		query += fmt.Sprintf(" AND $%d = ANY(tags)", argIdx)
		args = append(args, tag)
		argIdx++
	}
	if state := strings.TrimSpace(filter.State); state != "" {
		query += fmt.Sprintf(" AND state = $%d", argIdx)
		args = append(args, state)
		argIdx++
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		query += fmt.Sprintf(" AND (front ILIKE $%d OR back ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+q+"%")
		argIdx++
	}

	query += " ORDER BY created_at DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list cards: %w", err)
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

func (s *CardStore) CreateCard(ctx context.Context, userID, deckID string, input domain.CardInput) (domain.Card, error) {
	if err := s.verifyDeckOwnership(ctx, nil, userID, deckID); err != nil {
		return domain.Card{}, err
	}

	tags := normalizeTags(input.Tags)
	row := s.pool.QueryRow(ctx, `
		INSERT INTO cards (deck_id, user_id, front, back, tags, ease_factor)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING
			id, deck_id, user_id, front, back, tags,
			state, step, interval_days, ease_factor, repetitions, lapses,
			due_at, last_reviewed_at, created_at, updated_at
	`, deckID, userID, strings.TrimSpace(input.Front), strings.TrimSpace(input.Back), tags, srs.DefaultEaseFactor)

	return scanCard(row)
}

func (s *CardStore) BulkCreateCards(ctx context.Context, userID, deckID string, inputs []domain.CardInput) ([]domain.Card, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin bulk tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.verifyDeckOwnership(ctx, tx, userID, deckID); err != nil {
		return nil, err
	}

	created := make([]domain.Card, 0, len(inputs))
	for _, in := range inputs {
		front := strings.TrimSpace(in.Front)
		back := strings.TrimSpace(in.Back)
		if front == "" || back == "" {
			continue
		}
		tags := normalizeTags(in.Tags)
		row := tx.QueryRow(ctx, `
			INSERT INTO cards (deck_id, user_id, front, back, tags, ease_factor)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING
				id, deck_id, user_id, front, back, tags,
				state, step, interval_days, ease_factor, repetitions, lapses,
				due_at, last_reviewed_at, created_at, updated_at
		`, deckID, userID, front, back, tags, srs.DefaultEaseFactor)

		card, err := scanCard(row)
		if err != nil {
			return nil, fmt.Errorf("insert bulk card: %w", err)
		}
		created = append(created, card)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit bulk cards: %w", err)
	}
	return created, nil
}

func (s *CardStore) GetCard(ctx context.Context, userID, cardID string) (domain.Card, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT
			id, deck_id, user_id, front, back, tags,
			state, step, interval_days, ease_factor, repetitions, lapses,
			due_at, last_reviewed_at, created_at, updated_at
		FROM cards
		WHERE id = $1 AND user_id = $2
	`, cardID, userID)

	card, err := scanCard(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Card{}, domain.ErrNotFound
		}
		return domain.Card{}, fmt.Errorf("get card: %w", err)
	}
	return card, nil
}

func (s *CardStore) UpdateCard(ctx context.Context, userID, cardID string, input domain.CardInput) (domain.Card, error) {
	tags := normalizeTags(input.Tags)
	row := s.pool.QueryRow(ctx, `
		UPDATE cards
		SET front = $1, back = $2, tags = $3, updated_at = NOW()
		WHERE id = $4 AND user_id = $5
		RETURNING
			id, deck_id, user_id, front, back, tags,
			state, step, interval_days, ease_factor, repetitions, lapses,
			due_at, last_reviewed_at, created_at, updated_at
	`, strings.TrimSpace(input.Front), strings.TrimSpace(input.Back), tags, cardID, userID)

	card, err := scanCard(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Card{}, domain.ErrNotFound
		}
		return domain.Card{}, fmt.Errorf("update card: %w", err)
	}
	return card, nil
}

func (s *CardStore) DeleteCard(ctx context.Context, userID, cardID string) error {
	cmd, err := s.pool.Exec(ctx, `DELETE FROM cards WHERE id = $1 AND user_id = $2`, cardID, userID)
	if err != nil {
		return fmt.Errorf("delete card: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func scanCard(row pgx.Row) (domain.Card, error) {
	var c domain.Card
	err := row.Scan(
		&c.ID,
		&c.DeckID,
		&c.UserID,
		&c.Front,
		&c.Back,
		&c.Tags,
		&c.State,
		&c.Step,
		&c.IntervalDays,
		&c.EaseFactor,
		&c.Repetitions,
		&c.Lapses,
		&c.DueAt,
		&c.LastReviewedAt,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if err != nil {
		return domain.Card{}, err
	}
	if c.Tags == nil {
		c.Tags = []string{}
	}
	return c, nil
}

func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		clean := strings.ToLower(strings.TrimSpace(t))
		if clean == "" {
			continue
		}
		if _, exists := seen[clean]; !exists {
			seen[clean] = struct{}{}
			out = append(out, clean)
		}
	}
	return out
}
