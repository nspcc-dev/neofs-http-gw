FROM golang:1-alpine as builder

ARG BUILD=now
ARG VERSION=dev
ARG REPO=github.com/nspcc-dev/neofs-gw

ENV GOGC off
ENV CGO_ENABLED 0
ENV LDFLAGS "-w -s -X ${REPO}/Version=${VERSION} -X ${REPO}/Build=${BUILD}"

WORKDIR /src

COPY . /src

RUN go build -v -mod=vendor -ldflags "${LDFLAGS}" -o /go/bin/neofs-gw ./

# Executable image
FROM scratch

WORKDIR /

COPY --from=builder /go/bin/neofs-gw /bin/neofs-gw
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
