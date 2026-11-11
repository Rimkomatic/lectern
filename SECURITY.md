# Security

Please report a vulnerability privately, not in a public issue: on the GitHub
page of this repository open **Security → Report a vulnerability**.

Only the newest release gets fixes. Lectern is built for a home or small-team
network. If you make it reachable from the internet, put it behind a reverse
proxy that serves HTTPS (see "Behind a reverse proxy" in the README), choose
real passwords, and keep the image up to date (`docker compose pull`).
