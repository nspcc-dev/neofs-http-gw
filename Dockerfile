FROM golang:1-alpine as builder

ARG BUILD=now
ARG VERSION=dev
ARG REPO=github.com/nspcc-dev/neofs-gw

ENV GOGC off
ENV CGO_ENABLED 0
# add later -w -s
ENV LDFLAGS " -X main.Version=${VERSION} -compressdwarf=false"

WORKDIR /src

COPY . /src

RUN go build -v -mod=vendor -trimpath -gcflags=all="-N -l" -ldflags "${LDFLAGS} -X main.Build=$(date -u +%s%N)" -o /go/bin/neofs-gw ./

# Executable image
FROM alpine

WORKDIR /

COPY --from=builder /go/bin/neofs-gw /bin/neofs-gw
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ENTRYPOINT ["/bin/neofs-gw"]
