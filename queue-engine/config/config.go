package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all tunable parameters for the queue engine.
// Every value has a sensible default so the service runs locally
// without any environment variables, but can be overridden in prod.
type Config struct {
	// --- Server ---
	HTTPPort string

	// --- Redis ---
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// --- Queue behaviour ---
	// How many users are admitted from the queue per second.
	AdmitRatePerSec int
	// Maximum concurrent admitted (active) sessions — the "capacity" of the backend.
	MaxConcurrentAdmitted int
	// Seconds before an admitted session expires and frees a slot.
	AdmittedSessionTTL int
	// Seconds before a waiting user's queue entry expires (no heartbeat).
	WaitingTTL int

	// --- JWT ---
	JWTSecret string
	JWTTTL    int

	// --- Misc ---
	QueueName string // Redis key for the sorted set
	ShardCount int   // for future horizontal sharding
}

func Load() Config {
	return Config{
		HTTPPort:              getEnv("HTTP_PORT", "8080"),
		RedisAddr:             getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:         getEnv("REDIS_PASSWORD", ""),
		RedisDB:               getEnvInt("REDIS_DB", 0),
		AdmitRatePerSec:      getEnvInt("ADMIT_RATE_PER_SEC", 25),
		MaxConcurrentAdmitted: getEnvInt("MAX_CONCURRENT_ADMITTED", 200),
		AdmittedSessionTTL:   getEnvInt("ADMITTED_SESSION_TTL", 300),
		WaitingTTL:           getEnvInt("WAITING_TTL", 600),
		JWTSecret:            getEnv("JWT_SECRET", "dev-secret-change-me"),
		JWTTTL:               getEnvInt("JWT_TTL", 600),
		QueueName:            getEnv("QUEUE_NAME", "surge:queue"),
		ShardCount:           getEnvInt("SHARD_COUNT", 1),
	}
}

func (c Config) Validate() error {
	if c.AdmitRatePerSec < 1 {
		return fmt.Errorf("ADMIT_RATE_PER_SEC must be >= 1")
	}
	if c.MaxConcurrentAdmitted < 1 {
		return fmt.Errorf("MAX_CONCURRENT_ADMITTED must be >= 1")
	}
	if c.JWTSecret == "dev-secret-change-me" {
		fmt.Fprintln(os.Stderr, "WARNING: using default JWT secret — set JWT_SECRET in production")
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
