FROM busybox:1.28.4-glibc

COPY surbot /bin/surbot

RUN chmod +x /bin/surbot

ENV DISCORD_SECRET=$discord_secret

CMD ["/bin/surbot -t $DISCORD_SECRET"]