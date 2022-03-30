FROM golang:1.17.8-alpine3.15 as builder

RUN mkdir /build
WORKDIR /build
COPY .. .
RUN GO111MODULE=on CGO_ENABLED=0 GOOS=linux go build -a -o haproxy-reload-wrapper .
RUN wget https://github.com/Yelp/dumb-init/releases/download/v1.2.5/dumb-init_1.2.5_x86_64 -O dumb-init && \
    chmod +x dumb-init

FROM docker.io/haproxy:2.5

COPY --from=builder /build/haproxy-reload-wrapper /usr/local/sbin/haproxy-reload-wrapper
COPY --from=builder /build/dumb-init /usr/local/sbin/dumb-init

ENTRYPOINT ["/usr/local/sbin/dumb-init", "--", "/usr/local/sbin/haproxy-reload-wrapper", "-f", "/usr/local/etc/haproxy/haproxy.cfg"]
