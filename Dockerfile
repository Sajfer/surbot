FROM golang:1.19.3-alpine as builder

RUN apk add --no-cache \
    make=4.3-r0 \
    build-base=0.5-r3

WORKDIR /app
COPY . .
RUN make


FROM alpine:3.16 as runner

RUN adduser -s /bin/bash --disabled-password surbot && \
    apk add --no-cache \
        ffmpeg=5.0.1-r1 \
        curl=7.83.1-r4 \
        python3=3.10.5-r0  && \
    curl -L https://yt-dl.org/downloads/latest/youtube-dl -o /usr/local/bin/youtube-dl && \
    chmod a+rx /usr/local/bin/youtube-dl

# Add surbot
COPY --from=builder /app/bin/surbot /bin/surbot
RUN chmod +x /bin/surbot

USER surbot

# Run bot
CMD ["/bin/surbot"]
