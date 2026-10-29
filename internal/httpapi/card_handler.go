package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"fiszki-server/internal/auth"
	"fiszki-server/internal/domain"
	"fiszki-server/internal/extractor"
	"fiszki-server/internal/store"
)

const maxPhotoUploadBytes = 10 << 20 // 10 MB

type CardHandler struct {
	cards     *store.CardStore
	extractor extractor.CardExtractor
}

func NewCardHandler(cards *store.CardStore, ext extractor.CardExtractor) *CardHandler {
	return &CardHandler{cards: cards, extractor: ext}
}

func (h *CardHandler) ListCards(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	deckID := chi.URLParam(r, "deckID")

	filter := store.CardFilter{
		Tag:   r.URL.Query().Get("tag"),
		State: r.URL.Query().Get("state"),
		Query: r.URL.Query().Get("q"),
	}

	cards, err := h.cards.ListCards(r.Context(), userID, deckID, filter)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cards": cards})
}

func (h *CardHandler) CreateCard(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	deckID := chi.URLParam(r, "deckID")

	var input domain.CardInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, fmt.Errorf("%w: invalid JSON payload", domain.ErrValidation))
		return
	}

	if strings.TrimSpace(input.Front) == "" || strings.TrimSpace(input.Back) == "" {
		writeError(w, fmt.Errorf("%w: front and back are required", domain.ErrValidation))
		return
	}

	card, err := h.cards.CreateCard(r.Context(), userID, deckID, input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, card)
}

func (h *CardHandler) BulkCreateCards(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	deckID := chi.URLParam(r, "deckID")

	var req struct {
		Cards []domain.CardInput `json:"cards"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, fmt.Errorf("%w: invalid JSON payload", domain.ErrValidation))
		return
	}
	if len(req.Cards) == 0 {
		writeError(w, fmt.Errorf("%w: cards array cannot be empty", domain.ErrValidation))
		return
	}

	cards, err := h.cards.BulkCreateCards(r.Context(), userID, deckID, req.Cards)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"created_count": len(cards),
		"cards":         cards,
	})
}

func (h *CardHandler) GetCard(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	cardID := chi.URLParam(r, "cardID")

	card, err := h.cards.GetCard(r.Context(), userID, cardID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, card)
}

func (h *CardHandler) UpdateCard(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	cardID := chi.URLParam(r, "cardID")

	var input domain.CardInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, fmt.Errorf("%w: invalid JSON payload", domain.ErrValidation))
		return
	}
	if strings.TrimSpace(input.Front) == "" || strings.TrimSpace(input.Back) == "" {
		writeError(w, fmt.Errorf("%w: front and back are required", domain.ErrValidation))
		return
	}

	card, err := h.cards.UpdateCard(r.Context(), userID, cardID, input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, card)
}

func (h *CardHandler) DeleteCard(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	cardID := chi.URLParam(r, "cardID")

	if err := h.cards.DeleteCard(r.Context(), userID, cardID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ExtractFromPhoto handles zero-config ephemeral photo uploads and returns candidate {front, back} pairs.
func (h *CardHandler) ExtractFromPhoto(w http.ResponseWriter, r *http.Request) {
	if h.extractor == nil {
		writeError(w, fmt.Errorf("%w: photo extraction service is not configured", domain.ErrValidation))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoUploadBytes)
	if err := r.ParseMultipartForm(maxPhotoUploadBytes); err != nil {
		writeError(w, fmt.Errorf("%w: failed to parse multipart form (max 10MB): %v", domain.ErrValidation, err))
		return
	}
	if r.MultipartForm != nil {
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, fmt.Errorf("%w: 'image' file field is required", domain.ErrValidation))
		return
	}
	defer func() { _ = file.Close() }()

	imageBytes, err := io.ReadAll(file)
	if err != nil || len(imageBytes) == 0 {
		writeError(w, fmt.Errorf("%w: could not read uploaded image", domain.ErrValidation))
		return
	}

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(imageBytes)
	}
	if !isSupportedImageMIME(mimeType) {
		writeError(w, fmt.Errorf("%w: unsupported image type %q (use JPEG, PNG, or WebP)", domain.ErrValidation, mimeType))
		return
	}

	candidates, err := h.extractor.ExtractCardsFromImage(r.Context(), imageBytes, mimeType)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"count": len(candidates),
		"cards": candidates,
	})
}

func isSupportedImageMIME(mimeType string) bool {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/jpg", "image/png", "image/webp", "image/heic", "image/heif":
		return true
	default:
		return false
	}
}
