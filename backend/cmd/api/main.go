package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"story-tts/backend/internal/api"
	"story-tts/backend/internal/audio"
	"story-tts/backend/internal/config"
	"story-tts/backend/internal/provider"
	"story-tts/backend/internal/service"
	"story-tts/backend/internal/storage"
	"story-tts/backend/internal/telegram"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("khong tai duoc cau hinh: %v", err)
	}

	store, err := storage.NewStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("khong khoi tao duoc sqlite: %v", err)
	}
	defer store.Close()

	tgManager, err := telegram.NewManager(cfg.Telegram)
	if err != nil {
		log.Fatalf("khong khoi tao duoc telegram manager: %v", err)
	}

	manager, err := service.NewManager(cfg, store, provider.NewEdgeProvider(cfg.Edge), audio.NewFFmpegMerger(cfg.FFmpegPath), tgManager)
	if err != nil {
		log.Fatalf("khong khoi tao duoc service manager: %v", err)
	}

	router := api.NewRouter(manager)
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := manager.Start(ctx); err != nil {
			log.Printf("worker dung voi loi: %v", err)
		}
	}()

	go func() {
		log.Printf("story-tts backend dang nghe tai %s", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("khong the chay http server: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown http co loi: %v", err)
	}
}
