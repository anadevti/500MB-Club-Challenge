package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eleven/500mb-challenge/internal/handler"
	"github.com/eleven/500mb-challenge/internal/router"
	"github.com/eleven/500mb-challenge/internal/storage"
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	redisAddr := getEnv("REDIS_ADDR", "redis:6379")
	redisPassword := getEnv("REDIS_PASSWORD", "")
	listenAddr := getEnv("LISTEN_ADDR", ":8000")
	instanceID := getEnv("INSTANCE_ID", "")

	if instanceID == "" {
		hostname, _ := os.Hostname()
		instanceID = hostname
	}

	store := storage.NewRedisStore(redisAddr, redisPassword, 0)
	defer store.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for {
		if err := store.Ping(ctx); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			log.Fatal("timeout waiting for redis")
		case <-time.After(250 * time.Millisecond):
		}
	}
	log.Println("redis connected")

	h := handler.New(store, instanceID)
	mux := router.New(h)

	server := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		log.Printf("listening on %s (instance: %s)", listenAddr, instanceID)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("server stopped")
}
