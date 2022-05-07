FROM golang:1.18.1-alpine as builder

RUN apk add --no-cache \
    make=4.3-r0 \
    build-base=0.5-r2

WORKDIR /app
COPY . .
RUN make


FROM alpine:3.15 as runner

RUN adduser -s /bin/bash --disabled-password surbot && \
    apk add --no-cache \
        ffmpeg=4.4.1-r2 \
        curl=7.80.0-r0 \
        python2=2.7.18-r4  && \
    curl -L https://yt-dl.org/downloads/latest/youtube-dl -o /usr/local/bin/youtube-dl && \
    chmod a+rx /usr/local/bin/youtube-dl

# Add surbot
COPY --from=builder /app/bin/surbot /bin/surbot
RUN chmod +x /bin/surbot

USER surbot

# Run bot
CMD ["/bin/surbot"]
