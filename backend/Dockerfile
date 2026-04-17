FROM golang:1.25-bookworm AS builder

WORKDIR /app/backend

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/story-tts-backend ./cmd/api

FROM python:3.13-slim-bookworm

WORKDIR /app/backend

RUN apt-get update \
    && apt-get install -y --no-install-recommends ffmpeg ca-certificates \
    && python -m pip install --no-cache-dir --break-system-packages edge-tts==7.2.8 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/story-tts-backend /usr/local/bin/story-tts-backend

EXPOSE 18080

CMD ["story-tts-backend"]
