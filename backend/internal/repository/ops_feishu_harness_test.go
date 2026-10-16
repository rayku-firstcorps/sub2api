//go:build feishu_integration && !integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"
)

// A narrow PostgreSQL-only harness for Feishu outbox tests. The supplied DSN
// MUST refer to a disposable test database: the schema migrations are applied.
var integrationDB *sql.DB

func TestMain(m *testing.M) {
	os.Exit(runFeishuIntegration(m))
}

func runFeishuIntegration(m *testing.M) int {
	dsn := os.Getenv("SUB2API_FEISHU_TEST_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "SUB2API_FEISHU_TEST_DSN must point to a disposable PostgreSQL database")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var err error
	integrationDB, err = sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open test database:", err)
		return 1
	}
	defer func() { _ = integrationDB.Close() }()
	if err = integrationDB.PingContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "ping test database:", err)
		return 1
	}
	if err = ApplyMigrations(ctx, integrationDB); err != nil {
		fmt.Fprintln(os.Stderr, "apply test migrations:", err)
		return 1
	}
	return m.Run()
}
