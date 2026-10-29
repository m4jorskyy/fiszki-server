package httpapi

import (
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"fiszki-server/internal/auth"
	"fiszki-server/internal/domain"
	"fiszki-server/internal/store"
)

type AuthHandler struct {
	users  *store.UserStore
	tokens *auth.TokenManager
}

func NewAuthHandler(users *store.UserStore, tokens *auth.TokenManager) *AuthHandler {
	return &AuthHandler{users: users, tokens: tokens}
}

type authCredentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authTokensResponse struct {
	User         domain.User `json:"user"`
	AccessToken  string      `json:"access_token"`
	ExpiresAt    time.Time   `json:"expires_at"`
	RefreshToken string      `json:"refresh_token"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req authCredentialsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, fmt.Errorf("%w: invalid JSON payload", domain.ErrValidation))
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if _, err := mail.ParseAddress(email); err != nil {
		writeError(w, fmt.Errorf("%w: valid email is required", domain.ErrValidation))
		return
	}
	if len(req.Password) < 8 {
		writeError(w, fmt.Errorf("%w: password must be at least 8 characters", domain.ErrValidation))
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, err)
		return
	}

	user, err := h.users.CreateUser(r.Context(), email, hash)
	if err != nil {
		writeError(w, err)
		return
	}

	resp, err := h.issueTokens(r, user)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req authCredentialsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, fmt.Errorf("%w: invalid JSON payload", domain.ErrValidation))
		return
	}

	user, err := h.users.GetByEmail(r.Context(), req.Email)
	if err != nil {
		writeError(w, domain.ErrInvalidCredentials)
		return
	}

	if !auth.CheckPassword(req.Password, user.PasswordHash) {
		writeError(w, domain.ErrInvalidCredentials)
		return
	}

	resp, err := h.issueTokens(r, user)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.RefreshToken) == "" {
		writeError(w, fmt.Errorf("%w: refresh_token is required", domain.ErrValidation))
		return
	}

	tokenHash := auth.HashToken(strings.TrimSpace(req.RefreshToken))
	user, err := h.users.ConsumeRefreshToken(r.Context(), tokenHash)
	if err != nil {
		writeError(w, err)
		return
	}

	resp, err := h.issueTokens(r, user)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.RefreshToken) == "" {
		writeError(w, fmt.Errorf("%w: refresh_token is required", domain.ErrValidation))
		return
	}

	tokenHash := auth.HashToken(strings.TrimSpace(req.RefreshToken))
	_ = h.users.DeleteRefreshToken(r.Context(), tokenHash)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *AuthHandler) issueTokens(r *http.Request, user domain.User) (authTokensResponse, error) {
	accessToken, expiresAt, err := h.tokens.GenerateAccessToken(user)
	if err != nil {
		return authTokensResponse{}, err
	}

	rawRefresh, refreshHash, refreshExpiresAt, err := h.tokens.GenerateRefreshToken()
	if err != nil {
		return authTokensResponse{}, err
	}

	if err := h.users.SaveRefreshToken(r.Context(), user.ID, refreshHash, refreshExpiresAt); err != nil {
		return authTokensResponse{}, err
	}

	return authTokensResponse{
		User:         user,
		AccessToken:  accessToken,
		ExpiresAt:    expiresAt,
		RefreshToken: rawRefresh,
	}, nil
}
