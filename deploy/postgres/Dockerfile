FROM postgres:17-alpine

RUN apk add --no-cache su-exec \
    && sed -i 's/exec gosu postgres /exec su-exec postgres /' /usr/local/bin/docker-entrypoint.sh \
    && rm -f /usr/local/bin/gosu
