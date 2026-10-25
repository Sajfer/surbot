FROM golang:1.25.4-alpine AS builder

RUN apk add --no-cache \
  make \
  build-base

WORKDIR /app
COPY . .
RUN make build


FROM alpine:3.22 AS runner

RUN adduser -u 10000 -s /bin/bash --disabled-password surbot && \
  apk add --no-cache \
    ffmpeg \
    curl \
    python3 \
    deno && \
  curl -fL https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp && \
  chmod a+rx /usr/local/bin/yt-dlp

# yt-dlp downloads the audio (internal/plugins/music/youtube/ytdlp.go). YouTube changes
# often, so rebuild the image to pick up a newer yt-dlp. deno is the
# JavaScript runtime yt-dlp needs for YouTube.

# Add surbot
COPY --from=builder /app/bin/surbot /bin/surbot
RUN chmod +x /bin/surbot

USER surbot

# Run bot
CMD ["/bin/surbot"]
