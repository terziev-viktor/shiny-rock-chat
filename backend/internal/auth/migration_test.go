package auth

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestUsersMigrationEnforcesQRImmutability(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL migration test")
	}
	ctx := context.Background()
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)

	schema := "b1_migration_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := connection.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := connection.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	}()
	if _, err := connection.Exec(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../migrations/001_users.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}

	accountID := uuid.New()
	publicKey := make([]byte, 32)
	qrPayload := "shinyrock://v1/" + accountID.String() + "." + strings.Repeat("a", 64) + ".0123456789abcdef"
	if _, err := connection.Exec(ctx, `
		INSERT INTO users (account_id, public_key, key_fingerprint, qr_payload)
		VALUES ($1, $2, $3, $4)`, accountID, publicKey, strings.Repeat("a", 64), qrPayload); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `UPDATE users SET qr_payload = $1 WHERE account_id = $2`,
		qrPayload+"changed", accountID); err == nil {
		t.Fatal("migration allowed qr_payload to change")
	}
	if _, err := connection.Exec(ctx, `UPDATE users SET shiny_rock = 7 WHERE account_id = $1`, accountID); err != nil {
		t.Fatalf("migration blocked balance update: %v", err)
	}
	var balance int64
	if err := connection.QueryRow(ctx, `SELECT shiny_rock FROM users WHERE account_id = $1`, accountID).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != 7 {
		t.Fatalf("shiny_rock = %d, want 7", balance)
	}
}

func TestMigrationSQLHasOnlyExpectedBaseTables(t *testing.T) {
	migration, err := os.ReadFile("../../migrations/001_users.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"users", "auth_challenges"} {
		if !strings.Contains(string(migration), "CREATE TABLE "+table) {
			t.Errorf("migration does not create %s", table)
		}
	}
	if strings.Contains(string(migration), "scan_counters") || strings.Contains(string(migration), "scan_events") {
		t.Fatal("migration includes B3-owned schema")
	}
}
