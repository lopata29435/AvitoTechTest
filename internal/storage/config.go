package storage

import (
	"fmt"
	"os"
	"strings"
)

func readSecretEnv(key string) string {
	if file := os.Getenv(key + "_FILE"); file != "" {
		if b, err := os.ReadFile(file); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return os.Getenv(key)
}

type Config struct {
	URL string
}

func ConfigFromEnv() Config {
	if url := func() string {
		if f := os.Getenv("DATABASE_URL_FILE"); f != "" {
			if b, err := os.ReadFile(f); err == nil {
				return strings.TrimSpace(string(b))
			}
		}
		return os.Getenv("DATABASE_URL")
	}(); url != "" {
		return Config{URL: url}
	}
	user := readSecretEnv("POSTGRES_USER")
	pass := readSecretEnv("POSTGRES_PASSWORD")
	db := readSecretEnv("POSTGRES_DB")
	host := readSecretEnv("DB_HOST")
	port := readSecretEnv("DB_PORT")
	ssl := readSecretEnv("DB_SSLMODE")
	if user == "" || pass == "" || db == "" || host == "" || port == "" {
		return Config{URL: "postgres://postgres:postgres@localhost:5432/app?sslmode=disable"}
	}
	return Config{URL: fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, port, db, ssl)}
}
