ARG GOLANG_VERSION="1.24"

ARG TARGETOS
ARG TARGETARCH

FROM --platform=${TARGETARCH} docker.io/golang:${GOLANG_VERSION} as build

COPY go.* ./
COPY main.go ./
COPY collector ./collector
COPY errlimit ./errlimit 

ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build \
    -a -installsuffix cgo \
    -o /go/bin/digitalocean_exporter \
    ./main.go


FROM --platform=${TARGETARCH} gcr.io/distroless/static-debian12:latest

LABEL org.opencontainers.image.description="Prometheus Exporter for DigitalOcean"
LABEL org.opencontainers.image.source="https://github.com/DazWilkin/digitalocean_exporter"

COPY --from=build /go/bin/digitalocean_exporter /digitalocean_exporter

EXPOSE 9212

ENTRYPOINT ["/digitalocean_exporter"]
