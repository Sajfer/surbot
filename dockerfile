FROM golang:alpine as builder

RUN apk add --no-cache make build-base
WORKDIR /app
COPY . .
RUN make


FROM alpine:latest as runner

RUN adduser -s /bin/bash --disabled-password surbot
RUN apk add --no-cache ffmpeg curl python2
RUN curl -L https://yt-dl.org/downloads/latest/youtube-dl -o /usr/local/bin/youtube-dl
RUN chmod a+rx /usr/local/bin/youtube-dl

# Add surbot
COPY --from=builder /app/bin/surbot /bin/surbot
RUN chmod +x /bin/surbot

USER surbot

# Run bot
CMD ["/bin/surbot"]
