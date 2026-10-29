package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fiszki-server/internal/auth"
	"fiszki-server/internal/config"
	"fiszki-server/internal/db"
	"fiszki-server/internal/extractor"
	"fiszki-server/internal/httpapi"
	"fiszki-server/internal/store"
)

func main() {
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.ConnectAndMigrate(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database initialization failed: %v", err)
	}
	defer pool.Close()

	// Keep Supabase database active 24/7 (prevents Free Tier 7-day inactivity pause)
	db.StartKeepAlive(ctx, pool, 6*time.Hour)

	geminiExtractor, err := extractor.NewGeminiExtractor(ctx, cfg.GeminiAPIKey, cfg.GeminiModel)
	if err != nil {
		log.Fatalf("gemini extractor initialization failed: %v", err)
	}

	tokenManager := auth.NewTokenManager(cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)

	router := httpapi.NewRouter(httpapi.Dependencies{
		DBPool:        pool,
		UserStore:     store.NewUserStore(pool),
		DeckStore:     store.NewDeckStore(pool),
		CardStore:     store.NewCardStore(pool),
		StudyStore:    store.NewStudyStore(pool),
		TokenManager:  tokenManager,
		CardExtractor: geminiExtractor,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("fiszki-server listening on http://localhost:%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
