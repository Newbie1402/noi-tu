FROM alpine:3.21

RUN apk add --no-cache ca-certificates curl docker-cli git python3
COPY swarm-poller.sh /usr/local/bin/noitu-swarm-poller
RUN chmod 0755 /usr/local/bin/noitu-swarm-poller

ENTRYPOINT ["/usr/local/bin/noitu-swarm-poller"]
