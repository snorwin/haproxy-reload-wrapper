FROM golang:1.17.8-alpine3.15 as builder

RUN mkdir /build
WORKDIR /build
COPY .. .
RUN GO111MODULE=on CGO_ENABLED=0 GOOS=linux go build -a -o haproxy-reload-wrapper .

FROM docker.io/haproxy:%VERSION%

COPY --from=builder /build/haproxy-reload-wrapper /usr/local/sbin/haproxy-reload-wrapper

ENTRYPOINT ["/usr/local/sbin/haproxy-reload-wrapper", "-f", "/usr/local/etc/haproxy/haproxy.cfg"]
