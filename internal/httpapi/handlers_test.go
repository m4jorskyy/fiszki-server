package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"fiszki-server/internal/auth"
	"fiszki-server/internal/domain"
)

type mockExtractor struct {
	candidates []domain.ExtractedCardCandidate
	err        error
}

func (m *mockExtractor) ExtractCardsFromImage(_ context.Context, _ []byte, _ string) ([]domain.ExtractedCardCandidate, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.candidates, nil
}

func TestParseCardsFromCSV(t *testing.T) {
	csvData := "front,back,tags\nHola,Hello,spanish;greeting\nAdiós,Goodbye,\"spanish,basic\"\nIncompleteRowOnlyOneCol\n , , \n"
	cards, err := parseCardsFromCSV(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(cards))
	}
	if cards[0].Front != "Hola" || cards[0].Back != "Hello" || len(cards[0].Tags) != 2 {
		t.Fatalf("unexpected first card: %+v", cards[0])
	}
	if cards[1].Front != "Adiós" || len(cards[1].Tags) != 2 {
		t.Fatalf("unexpected second card: %+v", cards[1])
	}
}

func TestParseCardsFromJSON(t *testing.T) {
	t.Run("wrapped cards object", func(t *testing.T) {
		jsonData := `{"cards":[{"front":"Q1","back":"A1","tags":["t1"]}]}`
		cards, err := parseCardsFromJSON(strings.NewReader(jsonData))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cards) != 1 || cards[0].Front != "Q1" {
			t.Fatalf("unexpected parsed cards: %+v", cards)
		}
	})

	t.Run("direct JSON array", func(t *testing.T) {
		jsonData := `[{"front":"Q2","back":"A2"}]`
		cards, err := parseCardsFromJSON(strings.NewReader(jsonData))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cards) != 1 || cards[0].Front != "Q2" {
			t.Fatalf("unexpected parsed cards: %+v", cards)
		}
	})

	t.Run("invalid JSON returns validation error", func(t *testing.T) {
		_, err := parseCardsFromJSON(strings.NewReader(`not-json`))
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("expected ErrValidation, got %v", err)
		}
	})
}

func TestSanitizeFilename(t *testing.T) {
	if got := sanitizeFilename("  Spanish Verbs 101!@# "); got != "spanish-verbs-101" {
		t.Fatalf("expected spanish-verbs-101, got %q", got)
	}
	if got := sanitizeFilename("!!!"); got != "deck" {
		t.Fatalf("expected fallback 'deck', got %q", got)
	}
}

func TestSanitizeCSVCell(t *testing.T) {
	if got := sanitizeCSVCell("=CMD|' /C calc'!A0"); got != "'=CMD|' /C calc'!A0" {
		t.Fatalf("expected formula to be escaped with leading quote, got %q", got)
	}
	if got := sanitizeCSVCell("+SUM(A1:A2)"); got != "'+SUM(A1:A2)" {
		t.Fatalf("expected + formula to be escaped, got %q", got)
	}
	if got := sanitizeCSVCell("normal text"); got != "normal text" {
		t.Fatalf("expected normal text unchanged, got %q", got)
	}
}

func TestCloudflareRealIP(t *testing.T) {
	var seenRemoteAddr string
	h := cloudflareRealIP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenRemoteAddr = r.RemoteAddr
		w.WriteHeader(http.StatusOK)
	}))

	// 1. Loopback request with valid CF-Connecting-IP -> updates RemoteAddr
	req1 := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req1.RemoteAddr = "127.0.0.1:54321"
	req1.Header.Set("CF-Connecting-IP", "203.0.113.50")
	h.ServeHTTP(httptest.NewRecorder(), req1)
	if seenRemoteAddr != "203.0.113.50" {
		t.Fatalf("expected RemoteAddr 203.0.113.50 from loopback CF-Connecting-IP, got %q", seenRemoteAddr)
	}

	// 2. Non-loopback request attempting to spoof CF-Connecting-IP -> ignored
	req2 := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req2.RemoteAddr = "198.51.100.10:54321"
	req2.Header.Set("CF-Connecting-IP", "1.2.3.4")
	h.ServeHTTP(httptest.NewRecorder(), req2)
	if seenRemoteAddr != "198.51.100.10:54321" {
		t.Fatalf("expected spoofed CF-Connecting-IP from non-loopback to be ignored, got %q", seenRemoteAddr)
	}
}

func TestIsSupportedImageMIME(t *testing.T) {
	supported := []string{"image/jpeg", "IMAGE/PNG", "image/webp", "image/heic", "image/heif"}
	for _, m := range supported {
		if !isSupportedImageMIME(m) {
			t.Fatalf("expected MIME %q to be supported", m)
		}
	}
	unsupported := []string{"application/pdf", "text/plain", "image/svg+xml"}
	for _, m := range unsupported {
		if isSupportedImageMIME(m) {
			t.Fatalf("expected MIME %q to be unsupported", m)
		}
	}
}

func TestAuthCookies_SetAndClear(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.Header.Set("X-Forwarded-Proto", "https")

	now := time.Now().UTC()
	setAuthCookies(rec, req, "", "access.jwt.token", now.Add(15*time.Minute), "refresh.raw.token", now.Add(24*time.Hour))

	cookies := rec.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("expected 2 cookies, got %d", len(cookies))
	}

	var foundAccess, foundRefresh bool
	for _, c := range cookies {
		if c.Name == AccessTokenCookieName {
			foundAccess = true
			if c.Value != "access.jwt.token" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteNoneMode {
				t.Fatalf("unexpected access cookie: %+v", c)
			}
		}
		if c.Name == RefreshTokenCookieName {
			foundRefresh = true
			if c.Value != "refresh.raw.token" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteNoneMode {
				t.Fatalf("unexpected refresh cookie: %+v", c)
			}
		}
	}
	if !foundAccess || !foundRefresh {
		t.Fatalf("missing auth cookies: foundAccess=%v foundRefresh=%v", foundAccess, foundRefresh)
	}

	clearRec := httptest.NewRecorder()
	clearAuthCookies(clearRec, req, "")
	clearedCookies := clearRec.Result().Cookies()
	for _, c := range clearedCookies {
		if c.MaxAge != -1 || c.Value != "" {
			t.Fatalf("expected cleared cookie with MaxAge=-1 and empty value, got %+v", c)
		}
	}
}

func TestCORSMiddleware(t *testing.T) {
	allowed := []string{"https://my-app.vercel.app"}
	handler := corsMiddleware(allowed)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 1. Allowed localhost origin
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/decks", nil)
	req1.Header.Set("Origin", "http://localhost:3000")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatalf("expected allowed origin http://localhost:3000, got %q", rec1.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec1.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("expected credentials true")
	}

	// 2. Preflight OPTIONS request
	req2 := httptest.NewRequest(http.MethodOptions, "/api/v1/decks", nil)
	req2.Header.Set("Origin", "https://app.nihaowu.pl")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for OPTIONS preflight, got %d", rec2.Code)
	}
	if rec2.Header().Get("Access-Control-Allow-Origin") != "https://app.nihaowu.pl" {
		t.Fatalf("expected allowed origin https://app.nihaowu.pl, got %q", rec2.Header().Get("Access-Control-Allow-Origin"))
	}

	// 3. Disallowed origin
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/decks", nil)
	req3.Header.Set("Origin", "https://malicious-site.com")
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("expected no allow-origin for disallowed origin, got %q", rec3.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestWriteError_StatusMapping(t *testing.T) {
	cases := []struct {
		err        error
		wantStatus int
	}{
		{domain.ErrNotFound, http.StatusNotFound},
		{domain.ErrAlreadyExists, http.StatusConflict},
		{domain.ErrInvalidCredentials, http.StatusUnauthorized},
		{domain.ErrUnauthorized, http.StatusUnauthorized},
		{domain.ErrValidation, http.StatusBadRequest},
		{errors.New("unexpected db failure"), http.StatusInternalServerError},
	}

	for _, tc := range cases {
		rec := httptest.NewRecorder()
		writeError(rec, tc.err)
		if rec.Code != tc.wantStatus {
			t.Fatalf("for err %v expected status %d, got %d", tc.err, tc.wantStatus, rec.Code)
		}
	}
}

func TestExtractFromPhotoHandler(t *testing.T) {
	t.Run("successful photo extraction", func(t *testing.T) {
		ext := &mockExtractor{
			candidates: []domain.ExtractedCardCandidate{
				{Front: "mitochondria", Back: "powerhouse of the cell"},
			},
		}
		handler := NewCardHandler(nil, ext)

		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", `form-data; name="image"; filename="notes.png"`)
		h.Set("Content-Type", "image/png")
		part, err := mw.CreatePart(h)
		if err != nil {
			t.Fatalf("create part: %v", err)
		}
		_, _ = part.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
		_ = mw.Close()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/cards/extract-from-photo", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		rec := httptest.NewRecorder()

		handler.ExtractFromPhoto(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp struct {
			Count int                             `json:"count"`
			Cards []domain.ExtractedCardCandidate `json:"cards"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if resp.Count != 1 || resp.Cards[0].Front != "mitochondria" {
			t.Fatalf("unexpected response payload: %+v", resp)
		}
	})

	t.Run("nil extractor returns 400", func(t *testing.T) {
		handler := NewCardHandler(nil, nil)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/cards/extract-from-photo", nil)
		rec := httptest.NewRecorder()
		handler.ExtractFromPhoto(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 when extractor is nil, got %d", rec.Code)
		}
	})

	t.Run("unsupported mime type returns 400", func(t *testing.T) {
		ext := &mockExtractor{}
		handler := NewCardHandler(nil, ext)

		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", `form-data; name="image"; filename="doc.txt"`)
		h.Set("Content-Type", "text/plain")
		part, _ := mw.CreatePart(h)
		_, _ = part.Write([]byte("plain text file"))
		_ = mw.Close()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/cards/extract-from-photo", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		rec := httptest.NewRecorder()

		handler.ExtractFromPhoto(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for text/plain upload, got %d", rec.Code)
		}
	})
}

func TestHandlerInputValidation_And_RouterWiring(t *testing.T) {
	tm := auth.NewTokenManager("test-secret", 15*time.Minute, 24*time.Hour)
	router := NewRouter(Dependencies{
		TokenManager:  tm,
		CardExtractor: &mockExtractor{},
	})

	token, _, err := tm.GenerateAccessToken(domain.User{ID: "u-1", Email: "u@example.com"})
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	// 1. GET /healthz -> 200 OK
	reqHealth := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recHealth := httptest.NewRecorder()
	router.ServeHTTP(recHealth, reqHealth)
	if recHealth.Code != http.StatusOK {
		t.Fatalf("expected 200 from /healthz, got %d", recHealth.Code)
	}

	// 2. POST /api/v1/auth/register with invalid email -> 400
	reqReg := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"not-an-email","password":"password123"}`))
	recReg := httptest.NewRecorder()
	router.ServeHTTP(recReg, reqReg)
	if recReg.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid email, got %d", recReg.Code)
	}

	// 3. POST /api/v1/auth/register with short password -> 400
	reqShortPwd := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"valid@example.com","password":"short"}`))
	recShortPwd := httptest.NewRecorder()
	router.ServeHTTP(recShortPwd, reqShortPwd)
	if recShortPwd.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for short password, got %d", recShortPwd.Code)
	}

	// 3b. POST /api/v1/auth/register with >72 byte password -> 400 (not 500)
	longPwd := strings.Repeat("a", 73)
	reqLongPwd := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"valid@example.com","password":"`+longPwd+`"}`))
	recLongPwd := httptest.NewRecorder()
	router.ServeHTTP(recLongPwd, reqLongPwd)
	if recLongPwd.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for >72 byte password, got %d", recLongPwd.Code)
	}

	// 4. POST /api/v1/auth/refresh with empty token -> 400
	reqRefresh := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(`{"refresh_token":"  "}`))
	recRefresh := httptest.NewRecorder()
	router.ServeHTTP(recRefresh, reqRefresh)
	if recRefresh.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty refresh token, got %d", recRefresh.Code)
	}

	// 5. POST /api/v1/decks with empty name -> 400
	reqDeck := httptest.NewRequest(http.MethodPost, "/api/v1/decks", strings.NewReader(`{"name":"   "}`))
	reqDeck.Header.Set("Authorization", "Bearer "+token)
	recDeck := httptest.NewRecorder()
	router.ServeHTTP(recDeck, reqDeck)
	if recDeck.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty deck name, got %d", recDeck.Code)
	}

	// 6. POST /api/v1/decks/{deckID}/cards with missing back -> 400
	reqCard := httptest.NewRequest(http.MethodPost, "/api/v1/decks/deck-1/cards", strings.NewReader(`{"front":"OnlyFront","back":""}`))
	reqCard.Header.Set("Authorization", "Bearer "+token)
	recCard := httptest.NewRecorder()
	router.ServeHTTP(recCard, reqCard)
	if recCard.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty card back, got %d", recCard.Code)
	}

	// 7. POST /api/v1/decks/{deckID}/cards/bulk with empty cards slice -> 400
	reqBulk := httptest.NewRequest(http.MethodPost, "/api/v1/decks/deck-1/cards/bulk", strings.NewReader(`{"cards":[]}`))
	reqBulk.Header.Set("Authorization", "Bearer "+token)
	recBulk := httptest.NewRecorder()
	router.ServeHTTP(recBulk, reqBulk)
	if recBulk.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty bulk cards array, got %d", recBulk.Code)
	}

	// 8. POST /api/v1/study/cards/{cardID}/review with invalid rating -> 400
	reqReview := httptest.NewRequest(http.MethodPost, "/api/v1/study/cards/card-1/review", strings.NewReader(`{"rating":99}`))
	reqReview.Header.Set("Authorization", "Bearer "+token)
	recReview := httptest.NewRecorder()
	router.ServeHTTP(recReview, reqReview)
	if recReview.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid rating 99, got %d", recReview.Code)
	}
}
