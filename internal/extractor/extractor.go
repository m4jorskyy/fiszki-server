package extractor

import (
	"context"

	"fiszki-server/internal/domain"
)

type CardExtractor interface {
	ExtractCardsFromImage(ctx context.Context, imageBytes []byte, mimeType string) ([]domain.ExtractedCardCandidate, error)
}
