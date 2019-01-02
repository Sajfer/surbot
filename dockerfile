FROM golang:alpine

# Setup of discord token
ARG discord_secret
ENV DISCORD_SECRET=${discord_secret}
RUN echo $discord_secret

# Add surbot
COPY surbot /bin/surbot
RUN chmod +x /bin/surbot

# Run bot
CMD ["/bin/surbot", "-t", "$DISCORD_SECRET"]