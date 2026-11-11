package httpapi

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"fiszki-server/internal/auth"
	"fiszki-server/internal/extractor"
	"fiszki-server/internal/store"
)

type Dependencies struct {
	DBPool         *pgxpool.Pool
	UserStore      *store.UserStore
	DeckStore      *store.DeckStore
	CardStore      *store.CardStore
	StudyStore     *store.StudyStore
	TokenManager   *auth.TokenManager
	CardExtractor  extractor.CardExtractor
	AllowedOrigins []string
}

// cloudflareRealIP trusts CF-Connecting-IP only when the request arrives over loopback (local cloudflared tunnel).
func cloudflareRealIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			if cfIP := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); net.ParseIP(cfIP) != nil {
				r.RemoteAddr = cfIP
			}
		}
		next.ServeHTTP(w, r)
	})
}

func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && isAllowedOrigin(origin, allowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token, X-Requested-With")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isAllowedOrigin(origin string, allowedList []string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" || host == "127.0.0.1" {
		return true
	}
	if host == "nihaowu.pl" || strings.HasSuffix(host, ".nihaowu.pl") {
		return true
	}
	for _, a := range allowedList {
		if strings.EqualFold(a, origin) || strings.EqualFold(a, host) {
			return true
		}
	}
	return false
}

func NewRouter(deps Dependencies) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(cloudflareRealIP)
	r.Use(corsMiddleware(deps.AllowedOrigins))
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if deps.DBPool != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			var one int
			if err := deps.DBPool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db_unreachable"})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	authHandler := NewAuthHandler(deps.UserStore, deps.TokenManager)
	deckHandler := NewDeckHandler(deps.DeckStore, deps.CardStore)
	cardHandler := NewCardHandler(deps.CardStore, deps.CardExtractor)
	studyHandler := NewStudyHandler(deps.StudyStore)

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)
			r.Post("/refresh", authHandler.Refresh)
			r.Post("/logout", authHandler.Logout)

			r.Group(func(r chi.Router) {
				r.Use(auth.Middleware(deps.TokenManager))
				r.Get("/me", authHandler.Me)
			})
		})

		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware(deps.TokenManager))

			// Decks & Import/Export
			r.Get("/decks", deckHandler.ListDecks)
			r.Post("/decks", deckHandler.CreateDeck)
			r.Get("/decks/{deckID}", deckHandler.GetDeck)
			r.Put("/decks/{deckID}", deckHandler.UpdateDeck)
			r.Delete("/decks/{deckID}", deckHandler.DeleteDeck)
			r.Get("/decks/{deckID}/export", deckHandler.ExportDeck)
			r.Post("/decks/{deckID}/import", deckHandler.ImportDeck)

			// Cards in a deck
			r.Get("/decks/{deckID}/cards", cardHandler.ListCards)
			r.Post("/decks/{deckID}/cards", cardHandler.CreateCard)
			r.Post("/decks/{deckID}/cards/bulk", cardHandler.BulkCreateCards)

			// Individual Card operations & Photo Extraction
			r.Post("/cards/extract-from-photo", cardHandler.ExtractFromPhoto)
			r.Get("/cards/{cardID}", cardHandler.GetCard)
			r.Put("/cards/{cardID}", cardHandler.UpdateCard)
			r.Delete("/cards/{cardID}", cardHandler.DeleteCard)

			// Study & SM-2 reviews
			r.Get("/study/due", studyHandler.GetDueCards)
			r.Post("/study/cards/{cardID}/review", studyHandler.SubmitReview)

			// Statistics
			r.Get("/stats/overview", studyHandler.GetStatsOverview)
		})
	})

	return r
}
