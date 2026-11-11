# Releasing Lectern

GitHub Actions builds and publishes the image (`.github/workflows/release.yml`)
to `ghcr.io/rimkomatic/lectern` for `linux/amd64` and `linux/arm64`. Nothing is
built on your own machine, and nothing needs a secret: the workflow logs in to
the registry with the token GitHub gives every run.

| You do | The workflow publishes |
|---|---|
| push to `main` | `:edge` |
| push the tag `v1.2.3` | `:1.2.3`, `:1.2`, `:1` (not for `0.x`), `:latest`, and a GitHub release that carries `docker-compose.yml` and `env.example` |
| push the tag `v1.2.3-rc.1` | `:1.2.3-rc.1` and a pre-release (no `latest`) |

Tests (`ci.yml`) run first; if one fails, nothing is published.

## First time

1. On GitHub, create an empty public repository named `lectern` (no README, no
   licence: the source has both).
2. In the source folder run `make setup`. It fetches the dependencies and writes
   `go.sum`. **Commit `go.sum`:** the tests and the release stop without it.
3. Push:
   ```sh
   git init && git add . && git commit -m "Lectern"
   git branch -M main   # the workflows only run for main, whatever git init chose
   git remote add origin git@github.com:Rimkomatic/lectern.git
   git push -u origin main
   ```
4. Open the repository's **Actions** tab and watch "Build and publish". It runs
   four test jobs (Go, web, browser, Docker image) and then pushes `:edge`. If a
   job fails, fix that first; the logs say what.
5. Make the image public. A package starts private: GitHub → your profile →
   **Packages** → `lectern` → **Package settings** → **Change visibility** →
   **Public**. Check it from a machine that is not logged in to ghcr.io:
   `docker pull ghcr.io/rimkomatic/lectern:edge`.
6. Release it: `git tag v0.1.0 && git push origin v0.1.0`. When the run is
   green, `:0.1.0`, `:0.1` and `:latest` exist and the release page has the two
   files that the README's quick start downloads.
7. Try the README's quick start on a clean machine (or an empty folder), as a
   stranger would.

## Every release

1. Update `THIRD-PARTY-NOTICES.md` if you changed dependencies.
2. Say in the release notes if the release adds a database migration (they only
   go forward; people should back up before updating).
3. Tag and push: `git tag v0.2.0 && git push origin v0.2.0`.
4. Edit the generated release notes on GitHub if you want them nicer.

Version numbers: `MAJOR.MINOR.PATCH`. Use a `-rc.1` style suffix for a trial
release. The version is built into the program (`lectern -version`, and the
`/api/v1/setup` response); builds from `main` say `edge-<commit>`.

## Optional: also publish to Docker Hub

Create the repository on hub.docker.com and an access token with write access,
then in the GitHub repository (Settings → Secrets and variables → Actions) add
the secrets `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` and the variable
`DOCKERHUB_IMAGE` (for example `rimkomatic/lectern`). From the next run the same
tags go to both registries. Docker Hub limits anonymous pulls (100 per 6 hours
per IP address); ghcr.io does not, which is why the README uses it.

## Changing names

- **GitHub account or repository name:** the image path is lower case
  `ghcr.io/<owner>/<repo>`. It appears in `docker-compose.yml` and the README
  (and here and in `SECURITY.md`); the workflows take it from the repository
  they run in. `grep -rn rimkomatic .` finds the places.
- **Project name:** the Go module (`go.mod` and every import), the `LECTERN_*`
  settings, the `lectern_session` cookie, `web/src/lib/brand.ts` (the title in
  the interface), the browser storage keys `lectern-theme` and `lectern-volume`,
  and the container, volume and image names. Keep the volume name `lectern-mysql`
  unless you want a fresh database. A project-wide search and replace does the
  rest.
- **Copyright holder:** `LICENSE` says "The Lectern authors". Put your name there
  if you prefer.
