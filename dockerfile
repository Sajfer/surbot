FROM busybox:1.28.4-glibc

COPY surbot /bin/surbot

RUN chmod +x /bin/surbot

CMD ["/bin/surbot"]