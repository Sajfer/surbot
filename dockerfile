FROM busybox:glibc

# Setup of discord token
ARG discord_secret
ENV DISCORD_SECRET=${discord_secret}
RUN echo $discord_secret

# Add surbot
COPY deployment/docker_run.sh /docker_run.sh
RUN chmod +x /docker_run.sh
COPY surbot /bin/surbot
RUN chmod +x /bin/surbot

# Run bot
CMD ["/docker_run.sh"]