package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"fiszki-server/internal/domain"
)

type mockExtractor struct {
	candidates []domain.ExtractedCardCandidate
}

func (m *mockExtractor) ExtractCardsFromImage(_ context.Context, _ []byte, _ string) ([]domain.ExtractedCardCandidate, error) {
	return m.candidates, nil
}

func TestParseCardsFromCSV(t *testing.T) {
	csvData := "front,back,tags\nHola,Hello,spanish;greeting\nAdiós,Goodbye,spanish\n"
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
}

func TestParseCardsFromJSON(t *testing.T) {
	jsonData := `{"cards":[{"front":"Q1","back":"A1","tags":["t1"]}]}`
	cards, err := parseCardsFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cards) != 1 || cards[0].Front != "Q1" {
		t.Fatalf("unexpected parsed cards: %+v", cards)
	}
}

func TestExtractFromPhotoHandler(t *testing.T) {
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
	// Minimal PNG header bytes
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
}
