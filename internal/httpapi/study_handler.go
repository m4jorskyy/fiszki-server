package httpapi

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"fiszki-server/internal/auth"
	"fiszki-server/internal/domain"
	"fiszki-server/internal/store"
)

type StudyHandler struct {
	study *store.StudyStore
}

func NewStudyHandler(study *store.StudyStore) *StudyHandler {
	return &StudyHandler{study: study}
}

func (h *StudyHandler) GetDueCards(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	limit := 50
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		if n, err := strconv.Atoi(rawLimit); err == nil && n > 0 {
			limit = n
		}
	}

	filter := store.DueCardsFilter{
		DeckID: r.URL.Query().Get("deck_id"),
		Tag:    r.URL.Query().Get("tag"),
		Limit:  limit,
	}

	cards, err := h.study.GetDueCards(r.Context(), userID, filter)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count": len(cards),
		"cards": cards,
	})
}

func (h *StudyHandler) SubmitReview(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	cardID := chi.URLParam(r, "cardID")

	var req struct {
		Rating domain.Rating `json:"rating"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, fmt.Errorf("%w: invalid JSON payload", domain.ErrValidation))
		return
	}

	updatedCard, reviewLog, err := h.study.SubmitReview(r.Context(), userID, cardID, req.Rating)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"card":       updatedCard,
		"review_log": reviewLog,
	})
}

func (h *StudyHandler) GetStatsOverview(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	stats, err := h.study.GetOverviewStats(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
