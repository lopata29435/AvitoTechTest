package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/example/avito-pr-service/internal/server"
	"github.com/jackc/pgx/v5/pgxpool"
	postgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func applyMigrations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	dir := filepath.Join("..", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".sql" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if _, err := pool.Exec(context.Background(), string(b)); err != nil {
			t.Fatalf("apply %s: %v", e.Name(), err)
		}
	}
}

func setupDB(t *testing.T) (*pgxpool.Pool, func()) {
	if ext := os.Getenv("TEST_DATABASE_URL"); ext != "" {
		pool, err := pgxpool.New(context.Background(), ext)
		if err != nil {
			t.Fatalf("connect external db: %v", err)
		}
		applyMigrations(t, pool)
		return pool, func() { pool.Close() }
	}

	if runtime.GOOS == "windows" {
		t.Skip("Skipping testcontainers on Windows; set TEST_DATABASE_URL to run e2e tests against external Postgres")
	}

	ctx := context.Background()
	pgC, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("app"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
	)
	if err != nil {
		t.Fatalf("pg container: %v", err)
	}

	connStr, err := pgC.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("conn str: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}

	applyMigrations(t, pool)

	cleanup := func() {
		pool.Close()
		_ = pgC.Terminate(ctx)
	}
	return pool, cleanup
}

func TestFlow_CreateAssignMerge_AndStats(t *testing.T) {
	pool, cleanup := setupDB(t)
	defer cleanup()

	srv := httptest.NewServer(server.NewRouter(pool))
	defer srv.Close()

	teamBody := `{"team_name":"backend","members":[{"user_id":"u1","username":"Alice","is_active":true},{"user_id":"u2","username":"Bob","is_active":true},{"user_id":"u3","username":"Carol","is_active":true}]}`
	res, err := http.Post(srv.URL+"/team/add", "application/json", strings.NewReader(teamBody))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("team add status %d", res.StatusCode)
	}

	prBody := `{"pull_request_id":"pr-1","pull_request_name":"Test","author_id":"u1"}`
	res, err = http.Post(srv.URL+"/pullRequest/create", "application/json", strings.NewReader(prBody))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("pr create status %d", res.StatusCode)
	}

	res, err = http.Get(srv.URL + "/stats/assignments")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("stats status %d", res.StatusCode)
	}
	var stats struct {
		Assignments []struct {
			UserID string `json:"user_id"`
			Count  int    `json:"count"`
		} `json:"assignments"`
	}
	if err := json.NewDecoder(res.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	if len(stats.Assignments) == 0 {
		t.Fatalf("expected non-empty stats")
	}

	mergeBody := `{"pull_request_id":"pr-1"}`
	for i := 0; i < 2; i++ {
		res, err = http.Post(srv.URL+"/pullRequest/merge", "application/json", strings.NewReader(mergeBody))
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("merge status %d", res.StatusCode)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
