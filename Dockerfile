# Lectern in one image: the web app and the server, nothing else.
#
#   docker build -t lectern .            (the image alone)
#   docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
#
# Three stages: build the web app, compile the server with that web app inside
# it, then put the one binary on a small base image.
#
# The first two stages always run on the machine that builds (BUILDPLATFORM);
# the Go compiler cross-compiles for the target (TARGETOS, TARGETARCH). That is
# how one build produces linux/amd64 and linux/arm64 images without emulation:
# the last stage only copies files. Needs BuildKit, which Docker uses by default.

# ---- 1. Web app --------------------------------------------------------------
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
# Vite writes to ../internal/webui/dist, the folder the server embeds.
RUN npm run build

# ---- 2. Server ---------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS server
WORKDIR /src
# "go.sum*" so that a source tree without go.sum still builds: the line after the
# copies creates it then. The repository has it committed, and then it is used
# as is (and the release workflow refuses to publish without it).
COPY go.mod go.sum* ./
COPY cmd ./cmd
COPY internal ./internal
COPY --from=web /src/internal/webui/dist ./internal/webui/dist
RUN if [ ! -f go.sum ]; then go mod tidy; fi
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/lectern ./cmd/lectern

# ---- 3. Runtime image --------------------------------------------------------
FROM alpine:3.22
LABEL org.opencontainers.image.title="Lectern" \
      org.opencontainers.image.description="Self-hosted course server: folders of videos, PDFs and files with per-user progress" \
      org.opencontainers.image.licenses="MIT"
COPY --from=server /out/lectern /usr/local/bin/lectern
COPY LICENSE THIRD-PARTY-NOTICES.md /usr/share/licenses/lectern/
# The server reads the library and talks to MySQL; it writes no files, so it
# runs as an unprivileged user. docker-compose.yml sets the user id it really
# runs as, so that it can read your files.
USER 10001:10001
ENV LECTERN_LISTEN=:8080 \
    LECTERN_LIBRARIES=/library
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD ["/usr/local/bin/lectern", "-healthcheck"]
ENTRYPOINT ["/usr/local/bin/lectern"]
