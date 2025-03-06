# https://hub.docker.com/_/alpine/tags
ARG ALPINE_VERSION="3.21"

# https://pkgs.alpinelinux.org/package/edge/main/x86/ca-certificates
ARG CACERT_VERSION="20241121-r1"

# https://hub.docker.com/_/golang/tags
ARG GOLANG_VERSION="1.24-alpine${ALPINE_VERSION}"

ARG GOOS="linux"
ARG GOARCH="amd64"

FROM docker.io/golang:${GOLANG_VERSION} as build

COPY go.* ./
COPY main.go ./
COPY collector ./collector

ARG GOOS
ARG GOARCH

RUN CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} \
    go build \
    -a -installsuffix cgo \
    -o /go/bin/digitalocean_exporter \
    ./main.go


FROM docker.io/alpine:${ALPINE_VERSION}

ARG CACERT_VERSION

RUN apk add --no-cache ca-certificates=${CACERT_VERSION}

COPY --from=build /go/bin/digitalocean_exporter /usr/bin/digitalocean_exporter

EXPOSE 9212

ENTRYPOINT ["/usr/bin/digitalocean_exporter"]
