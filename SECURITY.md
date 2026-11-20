# Security

## Reporting a problem

Please report a vulnerability privately, not in a public issue: on the GitHub
page of this repository open **Security → Report a vulnerability**. Say what you
found, how to reproduce it, and which version you run (`lectern -version`). The
report is read by the maintainer, who will answer there, fix the problem in a
new release and, if you like, credit you in the release notes.

Only the newest release gets fixes.

## What Lectern is built for

A home or small-team network. If you make it reachable from the internet:

- put it behind a reverse proxy that serves HTTPS (see "Behind a reverse proxy"
  in the README) and set `LECTERN_TRUST_PROXY=true` there;
- choose long passwords. Accounts are created by an administrator, there is no
  sign-up form, and wrong passwords are rate limited (8 within 15 minutes, for
  each address and for each account name);
- create the administrator right after the first start. The setup page closes
  15 minutes after Lectern starts while nobody has an account
  (`LECTERN_SETUP_WINDOW`), but until you have made the account, anyone who can
  reach the page can. Or set `LECTERN_ADMIN_USER` and `LECTERN_ADMIN_PASSWORD`,
  and remove the password from `.env` afterwards;
- do not publish MySQL's port. In the supplied compose file only the app
  container can reach it;
- keep it up to date (`docker compose pull && docker compose up -d`).

## What it does to protect you

- Your library is mounted read-only, and files are only served from inside the
  library folders (a symbolic link must stay inside them too, unless you allow
  it with `LECTERN_FOLLOW_SYMLINKS`).
- The container runs as an ordinary user with a read-only file system, no
  Linux capabilities and `no-new-privileges`.
- Passwords are stored as PBKDF2-HMAC-SHA256 hashes (600,000 rounds). Logins
  and other changes must come from the site's own address (an `Origin` check),
  and the login cookie is `Secure` over HTTPS.
- The web app's content security policy allows only its own origin, so nothing
  is loaded from other sites. Markdown and HTML pages in a course are cleaned
  before they are shown (scripts, forms and frames removed), and pictures hosted
  elsewhere are never fetched.
- The server makes no connection to the internet: no analytics, no update check.
- Search engines are told to stay away (`robots.txt`, a `noindex` header and
  meta tag), in case the address ever leaks.

## What it does not protect against

- Whoever can run commands on the server, or in the container, controls
  Lectern: `lectern -reset-password` can set any account's password. Protect
  the server itself.
- Everyone with an account can open every course in the library: there is no
  per-course access control yet. Administrators can create, disable and delete
  accounts. Lectern does not show one person's progress to another, but whoever
  holds the database password can read it.
