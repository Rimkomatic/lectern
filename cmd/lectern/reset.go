package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"lectern/internal/accounts"
	"lectern/internal/config"
	"lectern/internal/db"
	"lectern/internal/store"
)

// runResetPassword is the -reset-password command: it needs the same
// environment as the server (the database settings), so inside Docker it runs
// as `docker compose exec app lectern -reset-password USER`. The result is the
// exit code: 0 done, 1 could not do it, 2 the command line or settings are wrong.
func runResetPassword(username string, fromStdin bool) int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		return 2
	}
	password := ""
	if fromStdin {
		if password, err = readPassword(os.Stdin); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	quiet := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	conn, err := db.Open(ctx, cfg.DB, 10*time.Second, quiet)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot reach the database:", err)
		return 1
	}
	defer conn.Close()
	return resetPassword(ctx, store.New(conn), username, password, os.Stdout, os.Stderr)
}

// resetPassword does the work and reports it; password is empty to get a random one.
func resetPassword(ctx context.Context, st *store.Store, username, password string, out, errOut io.Writer) int {
	res, err := accounts.ResetPassword(ctx, st, username, password)
	if err != nil {
		fmt.Fprintln(errOut, "cannot reset the password:", err)
		return 1
	}
	fmt.Fprintf(out, "The password for %q has been reset, and every device they were signed in on is signed out.\n", res.Username)
	if res.Generated {
		fmt.Fprintf(out, "\n  New password: %s\n\nSign in with it, then change it under Settings.\n", res.Password)
	}
	if res.Disabled {
		fmt.Fprintf(out, "\nNote: %q is disabled and cannot sign in until an administrator enables the account.\n", res.Username)
	}
	return 0
}

// readPassword reads the new password for -password-stdin: the first line,
// without its line ending and otherwise as it is.
func readPassword(r io.Reader) (string, error) {
	line, err := bufio.NewReader(io.LimitReader(r, 4096)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("-password-stdin: %w", err)
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return "", errors.New("-password-stdin: there is no password on standard input")
	}
	return line, nil
}
