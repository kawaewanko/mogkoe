# Build stage: CGO is required by github.com/hraban/opus.
FROM golang:1.23-bookworm AS build

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    pkg-config \
    libopus-dev \
    libopusfile-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# Copy module metadata first to retain dependency-layer caching.
COPY go.mod go.sum ./
COPY third_party/discordgo/go.mod third_party/discordgo/go.sum ./third_party/discordgo/
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build \
    -trimpath \
    -buildvcs=false \
    -ldflags="-s -w" \
    -o /out/mogkoe ./cmd/mogkoe

# Runtime stage: no Go toolchain, compiler, pkg-config, or -dev packages.
FROM debian:bookworm-slim AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    ffmpeg \
    libopus0 \
    libopusfile0 \
    && rm -rf /var/lib/apt/lists/* \
    && install -d -o 10001 -g 10001 -m 0700 /app/cache

COPY --from=build --chown=10001:10001 /out/mogkoe /usr/local/bin/mogkoe

WORKDIR /app
ENV CACHE_DIR=/app/cache \
    HOME=/tmp

# Fixed, unprivileged UID/GID; the cache volume is mounted at /app/cache.
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/mogkoe"]
