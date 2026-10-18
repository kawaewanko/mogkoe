FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mogkoe ./cmd/mogkoe

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg ca-certificates build-essential libopus-dev libopusfile-dev && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/mogkoe /usr/local/bin/mogkoe
WORKDIR /app
ENTRYPOINT ["mogkoe"]
