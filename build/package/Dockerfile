FROM golang:1.25.4-alpine AS builder

RUN apk add --no-cache \
  make \
  build-base

WORKDIR /app
COPY . .
RUN make build


FROM alpine:3.21 AS runner

RUN adduser -u 10000 -s /bin/bash --disabled-password surbot && \
  apk add --no-cache \
    ffmpeg \
    curl \
    python3 && \
  curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/youtube-dl && \
  chmod a+rx /usr/local/bin/youtube-dl

# Add surbot
COPY --from=builder /app/bin/surbot /bin/surbot
RUN chmod +x /bin/surbot

USER surbot

# Run bot
CMD ["/bin/surbot"]
