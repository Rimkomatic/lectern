# Changelog

What changed in each release of Lectern, newest first. Versions follow
[Semantic Versioning](https://semver.org/): while the first number is 0, a minor
version (0.2, 0.3, ...) may change how things behave, so read the entry before
updating. The database only moves forward when Lectern starts, so take a backup
first (the README's "Updating" shows how). There is no 0.1.1: that number was
skipped.

## 0.2.0 - 2026-11-14

Getting Lectern ready for people other than its author.

### Added

- **Reset a forgotten password** from the server:
  `docker compose exec app lectern -reset-password alex` prints a new random
  password once and signs that person out on every device. With
  `-password-stdin` you choose the password yourself.
- **Add to home screen.** On a phone, Lectern can be added to the home screen
  and opens like an app (Android and iPhone, with proper icons). It still needs
  your server; there is no offline mode.
- **A guard against going back.** An older version now refuses to start on a
  database that a newer one has already upgraded, instead of running on tables
  it does not understand. The log says what to do.
- **Search engines are told to stay away** (`robots.txt`, a `noindex` header and
  meta tag), in case the address of a server ever leaks.
- **README, security notes and project files for newcomers:** pictures, "What
  you get", questions and answers, a section for phones and one for forgotten
  passwords, a security page that says what Lectern does and does not protect
  against, a contributing guide, issue forms, and monthly dependency updates
  through Dependabot. `make screenshots` retakes the pictures from a made-up
  demo library.

### Changed

- **The setup page closes itself.** While nobody has an account, the page that
  creates the first administrator now works for 15 minutes after Lectern starts.
  Then it closes (the browser says why) until Lectern is restarted, so a server
  that nobody got to in time cannot be taken over later by whoever finds it.
  `LECTERN_SETUP_WINDOW` changes the time (`30m`, `2h`, `1d`) or turns the limit
  off (`forever`). Nothing changes once an account exists.
  **Updating from 0.1.x:** nothing to do. If you ever start Lectern with no
  account and miss the 15 minutes, restart it.
- The compose file and `.env.example` know the new setting.
- The Docker checks in CI now also test the home-screen manifest, the search
  engine signals, and the password reset command inside the real container.

## 0.1.3 - 2026-11-14

### Added

- **Drop course.** On a course page, "Drop course" deletes your own progress in
  that course (after a Yes/No question), so it shows as not started and leaves
  "Continue learning". The course, its files and other people's progress are
  untouched.

## 0.1.2 - 2026-11-13

### Added

- **A nightly scan.** The library is scanned once a day (03:00 by default;
  `LECTERN_SCAN_AT`, or `LECTERN_SCAN_INTERVAL` for "every so often"). The
  scan on start-up and Admin → Scan now stay.
- **Looser subtitle pairing.** A subtitle named only for its language
  (`English.srt`) goes with the single video in its folder; a subtitle that
  matches no video is listed under Admin → Library scan so it can be renamed.
- **Captions remembered.** **C** switches captions on and off, and the choice
  is kept on that device.
- **Markdown and HTML pages are shown formatted**, cleaned first, with a Source
  switch. Pictures stored beside the page in the course are shown too, and text
  files in UTF-8, UTF-16 and Windows-1252 are read correctly.

### Fixed

- The compose file did not hand `LECTERN_SCAN_AT` and `TZ` on to the app, so a
  scan time set in `.env` was ignored.

## 0.1.0 - 2026-11-12

The first release: a course library from folders, with per-person progress and
"continue where you left off", chapters for long videos, PDF, image, Markdown
and link lessons, covers, light and dark themes, administration, and a Docker
image for amd64 and arm64 started by one compose file.
