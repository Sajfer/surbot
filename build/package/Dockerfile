FROM golang:1.21.1-alpine as builder

RUN apk add --no-cache \
  make \
  build-base

WORKDIR /app
COPY . .
RUN make


FROM alpine:3.18 as runner

RUN adduser -u 1000 -s /bin/bash --disabled-password surbot && \
  apk add --no-cache \
    ffmpeg \
    curl \
    python3 && \
  curl -L https://yt-dl.org/downloads/latest/youtube-dl -o /usr/local/bin/youtube-dl && \
  chmod a+rx /usr/local/bin/youtube-dl

# Add surbot
COPY --from=builder /app/bin/surbot /bin/surbot
RUN chmod +x /bin/surbot

USER surbot

# Run bot
CMD ["/bin/surbot"]
