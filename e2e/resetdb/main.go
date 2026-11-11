// Command resetdb empties the test database (LECTERN_TEST_DB_*, default
// "lectern_test") so the browser smoke test in ../run.sh starts from a fresh,
// never-used server. It drops every table in that database: never point it at
// real data.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"lectern/internal/db"
	"lectern/internal/testdb"
)

func main() {
	cfg := testdb.Config()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	conn, err := db.Open(ctx, cfg, 30*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		fmt.Fprintln(os.Stderr, "resetdb:", err)
		os.Exit(1)
	}
	defer conn.Close()
	if err := testdb.Wipe(ctx, conn); err != nil {
		fmt.Fprintln(os.Stderr, "resetdb:", err)
		os.Exit(1)
	}
	fmt.Printf("emptied database %q on %s:%d\n", cfg.Name, cfg.Host, cfg.Port)
}
