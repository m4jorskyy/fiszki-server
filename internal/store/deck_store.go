package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fiszki-server/internal/domain"
)

type DeckStore struct {
	pool *pgxpool.Pool
}

func NewDeckStore(pool *pgxpool.Pool) *DeckStore {
	return &DeckStore{pool: pool}
}

func (s *DeckStore) ListDecks(ctx context.Context, userID string) ([]domain.Deck, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			d.id,
			d.user_id,
			d.name,
			d.description,
			d.created_at,
			d.updated_at,
			COUNT(c.id) AS total_cards,
			COUNT(c.id) FILTER (WHERE c.state = 'new') AS new_cards,
			COUNT(c.id) FILTER (WHERE c.state IN ('learning', 'relearning')) AS learning_cards,
			COUNT(c.id) FILTER (WHERE c.due_at <= NOW()) AS due_cards
		FROM decks d
		LEFT JOIN cards c ON c.deck_id = d.id
		WHERE d.user_id = $1
		GROUP BY d.id
		ORDER BY d.created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list decks: %w", err)
	}
	defer rows.Close()

	decks := make([]domain.Deck, 0)
	for rows.Next() {
		var d domain.Deck
		if err := rows.Scan(
			&d.ID,
			&d.UserID,
			&d.Name,
			&d.Description,
			&d.CreatedAt,
			&d.UpdatedAt,
			&d.Counts.TotalCards,
			&d.Counts.NewCards,
			&d.Counts.LearningCards,
			&d.Counts.DueCards,
		); err != nil {
			return nil, fmt.Errorf("scan deck: %w", err)
		}
		decks = append(decks, d)
	}
	return decks, rows.Err()
}

func (s *DeckStore) GetDeck(ctx context.Context, userID, deckID string) (domain.Deck, error) {
	var d domain.Deck
	err := s.pool.QueryRow(ctx, `
		SELECT
			d.id,
			d.user_id,
			d.name,
			d.description,
			d.created_at,
			d.updated_at,
			COUNT(c.id) AS total_cards,
			COUNT(c.id) FILTER (WHERE c.state = 'new') AS new_cards,
			COUNT(c.id) FILTER (WHERE c.state IN ('learning', 'relearning')) AS learning_cards,
			COUNT(c.id) FILTER (WHERE c.due_at <= NOW()) AS due_cards
		FROM decks d
		LEFT JOIN cards c ON c.deck_id = d.id
		WHERE d.id = $1 AND d.user_id = $2
		GROUP BY d.id
	`, deckID, userID).Scan(
		&d.ID,
		&d.UserID,
		&d.Name,
		&d.Description,
		&d.CreatedAt,
		&d.UpdatedAt,
		&d.Counts.TotalCards,
		&d.Counts.NewCards,
		&d.Counts.LearningCards,
		&d.Counts.DueCards,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Deck{}, domain.ErrNotFound
		}
		return domain.Deck{}, fmt.Errorf("get deck: %w", err)
	}
	return d, nil
}

func (s *DeckStore) CreateDeck(ctx context.Context, userID, name, description string) (domain.Deck, error) {
	var d domain.Deck
	err := s.pool.QueryRow(ctx, `
		INSERT INTO decks (user_id, name, description)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, name, description, created_at, updated_at
	`, userID, name, description).Scan(
		&d.ID,
		&d.UserID,
		&d.Name,
		&d.Description,
		&d.CreatedAt,
		&d.UpdatedAt,
	)
	if err != nil {
		return domain.Deck{}, fmt.Errorf("create deck: %w", err)
	}
	return d, nil
}

func (s *DeckStore) UpdateDeck(ctx context.Context, userID, deckID, name, description string) (domain.Deck, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		UPDATE decks
		SET name = $1, description = $2, updated_at = NOW()
		WHERE id = $3 AND user_id = $4
		RETURNING id
	`, name, description, deckID, userID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Deck{}, domain.ErrNotFound
		}
		return domain.Deck{}, fmt.Errorf("update deck: %w", err)
	}
	return s.GetDeck(ctx, userID, deckID)
}

func (s *DeckStore) DeleteDeck(ctx context.Context, userID, deckID string) error {
	cmd, err := s.pool.Exec(ctx, `DELETE FROM decks WHERE id = $1 AND user_id = $2`, deckID, userID)
	if err != nil {
		return fmt.Errorf("delete deck: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
