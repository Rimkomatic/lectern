# Contributing to Lectern

Thank you for thinking about it. Bug reports, ideas, documentation fixes and code are all welcome.

## Before you start

- **A bug or an idea?** Open an issue first, using the form that fits. For anything bigger than a small fix, agree on the idea there before writing code, so nobody's evening is wasted.
- **Scope.** Lectern plays files you already have, with progress per person. It does not transcode, download, or fetch anything from the internet, and the web app loads nothing from other sites. Changes that fit that are easy to accept; changes that turn it into something else are not.
- **Security problems** go through *Security → Report a vulnerability*, not a public issue (see `SECURITY.md`).

## Running it and the tests

The README's "Run from source" and "Development" sections have the steps. In short:

```sh
make setup              # once: Go and npm dependencies
make db db-test         # once: MySQL, plus an empty lectern_test database
make test               # gofmt, go vet, Go tests, svelte-check, web tests, colour contrast
make e2e                # the browser test; needs ffmpeg, Python with playwright, and Chrome
```

`make test` and `make e2e` empty the `lectern_test` database. Both also run in GitHub Actions on every pull request.

## What a good change looks like

- **Small and focused**, with a message that says why, not only what.
- **Tests.** A bug fix comes with a test that fails without the fix. New behaviour comes with tests at the level where it lives (Go test, web unit test, and a check in `e2e/smoke.py` if you can see it in the browser).
- **Code style.** `gofmt` for Go, and `npm run check` (svelte-check) for the web app. Comments explain why, in plain words.
- **The docs move with the code.** A new setting goes into `.env.example`, the compose file and the README's settings table. A user-visible change goes into `CHANGELOG.md`.
- **Database changes** are a new file in `internal/db/migrations/` (`NNNN_name.sql`). Migrations only go forward, so keep each one small and safe to run twice.
- **Dependencies.** The Go server has one (the MySQL driver) on purpose. Think twice before adding another, and update `THIRD-PARTY-NOTICES.md` when anything changes.

## Pull requests

Open them against `main`. CI has to be green. Expect questions and small requests for changes; that is how the code stays easy to read for the next person.

By contributing you agree that your work is released under the project's MIT licence.
