FROM golang:1.25.5-alpine3.23 as builder
ARG VERSION
ARG HASH

RUN mkdir /build
WORKDIR /build
COPY .. .
RUN GO111MODULE=on CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.Version=${VERSION} -X main.Hash=${HASH}" -a -o haproxy-reload-wrapper .

FROM docker.io/haproxy:%VERSION%

COPY --from=builder /build/haproxy-reload-wrapper /usr/local/sbin/haproxy-reload-wrapper

ENTRYPOINT ["/usr/local/sbin/haproxy-reload-wrapper", "-f", "/usr/local/etc/haproxy/haproxy.cfg"]
