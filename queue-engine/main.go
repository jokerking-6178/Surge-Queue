package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/surge/queue-engine/api"
	"github.com/surge/queue-engine/config"
	"github.com/surge/queue-engine/queue"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config invalid: %v", err)
	}

	log.Printf("starting surge-queue-engine on :%s", cfg.HTTPPort)
	log.Printf("  admit rate: %d/s, max concurrent: %d, session TTL: %ds",
		cfg.AdmitRatePerSec, cfg.MaxConcurrentAdmitted, cfg.AdmittedSessionTTL)

	// Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		PoolSize:     128,
		MinIdleConns: 16,
	})

	// Wait for Redis with a bounded retry loop.
	if err := waitForRedis(rdb, 30, time.Second); err != nil {
		log.Fatalf("redis unavailable: %v", err)
	}
	log.Printf("connected to redis at %s", cfg.RedisAddr)

	// Start the queue manager (launches background admitter + cleanup goroutines).
	qm := queue.NewManager(rdb, cfg)

	h := &api.Handlers{QM: qm}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"}, // tighten in production via ALLOWED_ORIGINS
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	h.RegisterRoutes(r)

	// Prometheus metrics endpoint
	r.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.HTTPPort),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Wait for interrupt signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("forced shutdown: %v", err)
	}
	_ = rdb.Close()
	log.Println("bye")
}

// waitForRedis retries the connection with a bounded loop.
func waitForRedis(rdb *redis.Client, maxRetries int, interval time.Duration) error {
	for i := 0; i < maxRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := rdb.Ping(ctx).Err()
		cancel()
		if err == nil {
			return nil
		}
		log.Printf("redis ping attempt %d/%d failed: %v", i+1, maxRetries, err)
		time.Sleep(interval)
	}
	return fmt.Errorf("redis not reachable after %d retries", maxRetries)
}
