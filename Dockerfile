FROM alpine:3.20.2

WORKDIR /app

COPY sharefile /usr/bin/
ENTRYPOINT ["/usr/bin/sharefile"]
