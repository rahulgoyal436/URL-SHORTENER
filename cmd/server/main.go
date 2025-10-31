package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"url-shortener/internal/handler"
	"url-shortener/internal/repository"
	"url-shortener/internal/service"

	"github.com/gorilla/handlers"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"

	"github.com/redis/go-redis/v9"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file loaded")
	}

	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		log.Fatal("DATABASE_DSN not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}

	if err := db.Ping(); err != nil {
		log.Fatal("db ping:", err)
	}

	// Redis optional
	var rdb *redis.Client
	redisURL := os.Getenv("REDIS_ADDR")
	if redisURL != "" {
		rdb = redis.NewClient(&redis.Options{
			Addr: redisURL,
		})
		if err := rdb.Ping(context.Background()).Err(); err != nil {
			log.Println("redis ping failed:", err)
			rdb = nil
		} else {
			log.Println("redis connected")
		}
	}

	repo := repository.NewRepo(db)
	svc := service.NewService(repo, rdb)
	h := handler.NewHandler(svc)

	r := h.Routes()

	// CORS
	allowed := handlers.AllowedOrigins([]string{"*"})
	allowedHeaders := handlers.AllowedHeaders([]string{"Content-Type", "X-Admin-Token"})
	allowedMethods := handlers.AllowedMethods([]string{"GET", "POST", "OPTIONS"})

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      handlers.CORS(allowed, allowedHeaders, allowedMethods)(r),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// graceful shutdown
	go func() {
		log.Printf("server listening on %s\n", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server shutdown: %v", err)
	}

	if rdb != nil {
		_ = rdb.Close()
	}
	_ = db.Close()
	log.Println("server gracefully stopped")
}
