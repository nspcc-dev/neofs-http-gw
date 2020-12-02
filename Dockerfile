FROM golang:1 as builder


ENV GOGC off
ENV CGO_ENABLED 0

RUN set -x \
    && apt update \
    && apt install -y upx-ucl

WORKDIR /src
COPY . /src

ARG VERSION=dev
ENV LDFLAGS "-w -s -X main.Version=${VERSION}"
RUN set -x \
    && go build \
      -v \
      -mod=vendor \
      -trimpath \
      -ldflags "${LDFLAGS} -X main.Build=$(date -u +%s%N) -X main.Prefix=HTTP_GW" \
      -o /go/bin/neofs-gw ./ \
    && upx -3 /go/bin/neofs-gw

# Executable image
FROM scratch

WORKDIR /

COPY --from=builder /go/bin/neofs-gw /bin/neofs-gw
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ENTRYPOINT ["/bin/neofs-gw"]
