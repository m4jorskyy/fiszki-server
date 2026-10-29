package extractor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"fiszki-server/internal/domain"
)

const extractionSystemPrompt = `You are an expert flashcard extraction engine.
Analyze the provided photo (which may be a textbook page, vocabulary table, glossary, handwritten study notes, slide, or physical flashcards) and extract study flashcards as plain-text front and back pairs.

Rules:
1. If the image contains a vocabulary list or translation table, put the term/phrase on "front" and its translation/definition on "back".
2. If the image contains definitions, concepts, or formulas, put the question/term/concept on "front" and the explanation/answer/formula on "back".
3. If the image contains study notes or prose paragraphs, synthesize concise, high-value question ("front") and answer ("back") pairs covering the key facts.
4. Keep "front" and "back" plain text only (no markdown formatting or HTML).
5. Return ONLY valid JSON matching the requested schema.`

type GeminiExtractor struct {
	client *genai.Client
	model  string
}

type geminiResponsePayload struct {
	Cards []domain.ExtractedCardCandidate `json:"cards"`
}

func NewGeminiExtractor(ctx context.Context, apiKey string, model string) (*GeminiExtractor, error) {
	if strings.TrimSpace(apiKey) == "" {
		return &GeminiExtractor{client: nil, model: model}, nil
	}
	if model == "" {
		model = "gemini-2.5-flash"
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("init gemini client: %w", err)
	}

	return &GeminiExtractor{
		client: client,
		model:  model,
	}, nil
}

func (g *GeminiExtractor) ExtractCardsFromImage(ctx context.Context, imageBytes []byte, mimeType string) ([]domain.ExtractedCardCandidate, error) {
	if g == nil || g.client == nil {
		return nil, fmt.Errorf("%w: GEMINI_API_KEY is not configured on the server", domain.ErrValidation)
	}

	schema := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"cards": {
				Type: genai.TypeArray,
				Items: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"front": {
							Type:        genai.TypeString,
							Description: "Plain text question, term, or prompt on the front of the flashcard",
						},
						"back": {
							Type:        genai.TypeString,
							Description: "Plain text answer, translation, or definition on the back of the flashcard",
						},
					},
					Required: []string{"front", "back"},
				},
			},
		},
		Required: []string{"cards"},
	}

	contents := []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				genai.NewPartFromBytes(imageBytes, mimeType),
				genai.NewPartFromText("Extract all flashcards from this image."),
			},
		},
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{genai.NewPartFromText(extractionSystemPrompt)},
		},
		ResponseMIMEType: "application/json",
		ResponseSchema:   schema,
	}

	resp, err := g.client.Models.GenerateContent(ctx, g.model, contents, config)
	if err != nil {
		return nil, fmt.Errorf("gemini generate content: %w", err)
	}

	rawJSON := strings.TrimSpace(resp.Text())
	if rawJSON == "" {
		return []domain.ExtractedCardCandidate{}, nil
	}

	var parsed geminiResponsePayload
	if err := json.Unmarshal([]byte(rawJSON), &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal gemini structured output: %w", err)
	}

	cleaned := make([]domain.ExtractedCardCandidate, 0, len(parsed.Cards))
	for _, c := range parsed.Cards {
		front := strings.TrimSpace(c.Front)
		back := strings.TrimSpace(c.Back)
		if front != "" && back != "" {
			cleaned = append(cleaned, domain.ExtractedCardCandidate{
				Front: front,
				Back:  back,
			})
		}
	}

	return cleaned, nil
}
