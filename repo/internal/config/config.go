// Package config loads runtime configuration for the HarborClass server.
// All values are sourced from environment variables so the service can be
// started purely via `docker compose up` without on-host bootstrapping.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds server-wide runtime configuration.
type Config struct {
	HTTPAddr         string
	DatabaseURL      string
	MigrationsPath   string
	SeedDemoData     bool
	EncryptionKey    string
	ReminderDailyCap int
	RetryMaxAttempts int
	RetryBaseDelay   time.Duration
	PickupCutoffHour int
}

// Load reads configuration from environment variables, applying defaults
// suitable for the bundled docker-compose deployment.
func Load() Config {
	return Config{
		HTTPAddr:         getenv("HARBORCLASS_HTTP_ADDR", ":8080"),
		DatabaseURL:      getenv("HARBORCLASS_DB_URL", "postgres://harbor:harbor@db:5432/harborclass?sslmode=disable"),
		MigrationsPath:   getenv("HARBORCLASS_MIGRATIONS", "internal/store/migrations.sql"),
		SeedDemoData:     getenvBool("HARBORCLASS_SEED", true),
		EncryptionKey:    getenv("HARBORCLASS_ENCRYPTION_KEY", "dev-only-encryption-key-change-me-32"),
		ReminderDailyCap: getenvInt("HARBORCLASS_REMINDER_CAP", 3),
		RetryMaxAttempts: getenvInt("HARBORCLASS_RETRY_MAX", 5),
		RetryBaseDelay:   time.Duration(getenvInt("HARBORCLASS_RETRY_BASE_MS", 500)) * time.Millisecond,
		PickupCutoffHour: getenvInt("HARBORCLASS_PICKUP_CUTOFF_HOUR", 20),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
