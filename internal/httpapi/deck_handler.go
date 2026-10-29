package httpapi

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"fiszki-server/internal/auth"
	"fiszki-server/internal/domain"
	"fiszki-server/internal/store"
)

type DeckHandler struct {
	decks *store.DeckStore
	cards *store.CardStore
}

func NewDeckHandler(decks *store.DeckStore, cards *store.CardStore) *DeckHandler {
	return &DeckHandler{decks: decks, cards: cards}
}

type deckPayload struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *DeckHandler) ListDecks(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	decks, err := h.decks.ListDecks(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"decks": decks})
}

func (h *DeckHandler) CreateDeck(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	var req deckPayload
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, fmt.Errorf("%w: invalid JSON payload", domain.ErrValidation))
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, fmt.Errorf("%w: deck name is required", domain.ErrValidation))
		return
	}

	deck, err := h.decks.CreateDeck(r.Context(), userID, name, strings.TrimSpace(req.Description))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, deck)
}

func (h *DeckHandler) GetDeck(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	deckID := chi.URLParam(r, "deckID")

	deck, err := h.decks.GetDeck(r.Context(), userID, deckID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deck)
}

func (h *DeckHandler) UpdateDeck(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	deckID := chi.URLParam(r, "deckID")

	var req deckPayload
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, fmt.Errorf("%w: invalid JSON payload", domain.ErrValidation))
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, fmt.Errorf("%w: deck name is required", domain.ErrValidation))
		return
	}

	deck, err := h.decks.UpdateDeck(r.Context(), userID, deckID, name, strings.TrimSpace(req.Description))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deck)
}

func (h *DeckHandler) DeleteDeck(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	deckID := chi.URLParam(r, "deckID")

	if err := h.decks.DeleteDeck(r.Context(), userID, deckID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *DeckHandler) ExportDeck(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	deckID := chi.URLParam(r, "deckID")
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "json"
	}

	deck, err := h.decks.GetDeck(r.Context(), userID, deckID)
	if err != nil {
		writeError(w, err)
		return
	}

	cards, err := h.cards.ListCards(r.Context(), userID, deckID, store.CardFilter{})
	if err != nil {
		writeError(w, err)
		return
	}

	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, sanitizeFilename(deck.Name)))
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"front", "back", "tags"})
		for _, c := range cards {
			_ = cw.Write([]string{c.Front, c.Back, strings.Join(c.Tags, ";")})
		}
		cw.Flush()

	case "json":
		exportItems := make([]domain.CardInput, 0, len(cards))
		for _, c := range cards {
			exportItems = append(exportItems, domain.CardInput{
				Front: c.Front,
				Back:  c.Back,
				Tags:  c.Tags,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"deck": map[string]string{
				"name":        deck.Name,
				"description": deck.Description,
			},
			"cards": exportItems,
		})

	default:
		writeError(w, fmt.Errorf("%w: format must be 'json' or 'csv'", domain.ErrValidation))
	}
}

func (h *DeckHandler) ImportDeck(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	deckID := chi.URLParam(r, "deckID")
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		if strings.Contains(r.Header.Get("Content-Type"), "csv") {
			format = "csv"
		} else {
			format = "json"
		}
	}

	var inputs []domain.CardInput
	var err error

	switch format {
	case "csv":
		inputs, err = parseCardsFromCSV(r.Body)
	case "json":
		inputs, err = parseCardsFromJSON(r.Body)
	default:
		err = fmt.Errorf("%w: format must be 'json' or 'csv'", domain.ErrValidation)
	}
	if err != nil {
		writeError(w, err)
		return
	}

	if len(inputs) == 0 {
		writeError(w, fmt.Errorf("%w: no valid cards found in import payload", domain.ErrValidation))
		return
	}

	created, err := h.cards.BulkCreateCards(r.Context(), userID, deckID, inputs)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"imported_count": len(created),
		"cards":          created,
	})
}

func parseCardsFromCSV(r io.Reader) ([]domain.CardInput, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true

	records, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%w: invalid CSV format: %v", domain.ErrValidation, err)
	}

	var inputs []domain.CardInput
	for i, row := range records {
		if len(row) < 2 {
			continue
		}
		front := strings.TrimSpace(row[0])
		back := strings.TrimSpace(row[1])
		// Skip optional header row
		if i == 0 && strings.EqualFold(front, "front") && strings.EqualFold(back, "back") {
			continue
		}
		if front == "" || back == "" {
			continue
		}
		var tags []string
		if len(row) >= 3 && strings.TrimSpace(row[2]) != "" {
			for _, part := range strings.FieldsFunc(row[2], func(r rune) bool { return r == ';' || r == ',' }) {
				if t := strings.TrimSpace(part); t != "" {
					tags = append(tags, t)
				}
			}
		}
		inputs = append(inputs, domain.CardInput{
			Front: front,
			Back:  back,
			Tags:  tags,
		})
	}
	return inputs, nil
}

func parseCardsFromJSON(r io.Reader) ([]domain.CardInput, error) {
	body, err := io.ReadAll(io.LimitReader(r, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read request body: %v", domain.ErrValidation, err)
	}

	var wrapper struct {
		Cards []domain.CardInput `json:"cards"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil && wrapper.Cards != nil {
		return wrapper.Cards, nil
	}

	var direct []domain.CardInput
	if err := json.Unmarshal(body, &direct); err == nil {
		return direct, nil
	}

	return nil, fmt.Errorf("%w: expected JSON object with 'cards' array or a JSON array of cards", domain.ErrValidation)
}

func sanitizeFilename(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else if r == ' ' {
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "deck"
	}
	return b.String()
}
