FROM golang:1.27.3-alpine AS builder

WORKDIR /app
# Dependencies first, so a code change does not download them again.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/surbot .


FROM alpine:3.24 AS runner

# yt-dlp looks up and downloads the audio (internal/plugins/music/youtube).
# YouTube changes often and old yt-dlp versions stop working, so keep this
# current; Renovate bumps it (renovate.json). deno is the JavaScript runtime
# yt-dlp needs for YouTube.
# renovate: datasource=github-releases depName=yt-dlp/yt-dlp
ARG YTDLP_VERSION=2026.10.26

RUN apk add --no-cache ffmpeg python3 deno && \
  apk add --no-cache --virtual .fetch curl && \
  cd /tmp && \
  curl -fsSLO "https://github.com/yt-dlp/yt-dlp/releases/download/${YTDLP_VERSION}/yt-dlp" && \
  curl -fsSLO "https://github.com/yt-dlp/yt-dlp/releases/download/${YTDLP_VERSION}/SHA2-256SUMS" && \
  grep ' yt-dlp$' SHA2-256SUMS > yt-dlp.sha256 && \
  sha256sum -c yt-dlp.sha256 && \
  install -m 0755 yt-dlp /usr/local/bin/yt-dlp && \
  rm yt-dlp SHA2-256SUMS yt-dlp.sha256 && \
  apk del .fetch && \
  adduser -D -u 10000 -s /sbin/nologin surbot

COPY --from=builder /out/surbot /bin/surbot

USER surbot

CMD ["/bin/surbot"]
