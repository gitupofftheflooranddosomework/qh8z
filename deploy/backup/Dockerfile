FROM alpine:3.24
RUN apk add --no-cache ca-certificates postgresql17-client restic tzdata
COPY backup.sh restore.sh /usr/local/bin/
RUN chmod 0555 /usr/local/bin/backup.sh /usr/local/bin/restore.sh
ENTRYPOINT ["/usr/local/bin/backup.sh"]
