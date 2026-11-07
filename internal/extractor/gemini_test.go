package extractor

import (
	"context"
	"errors"
	"testing"

	"fiszki-server/internal/domain"
)

func TestGeminiExtractor_MissingAPIKey(t *testing.T) {
	ext, err := NewGeminiExtractor(context.Background(), "", "")
	if err != nil {
		t.Fatalf("expected no error when creating extractor without API key, got %v", err)
	}

	_, err = ext.ExtractCardsFromImage(context.Background(), []byte("fake-image"), "image/png")
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation when API key is missing, got %v", err)
	}
}

func TestGeminiExtractor_NilReceiver(t *testing.T) {
	var ext *GeminiExtractor
	_, err := ext.ExtractCardsFromImage(context.Background(), []byte("fake"), "image/png")
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation on nil receiver, got %v", err)
	}
}
