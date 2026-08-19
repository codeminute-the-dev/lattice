ARG GO_VERSION=1.26.6
ARG ALPINE_VERSION=3.24
ARG COREDNS_VERSION=1.11.3

FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS builder

RUN apk --no-cache add ca-certificates libcap git make

ARG COREDNS_VERSION

RUN git clone --depth 1 --branch v${COREDNS_VERSION} \
    https://github.com/coredns/coredns /coredns

COPY . /lattice

WORKDIR /coredns

RUN echo "dnsseed:github.com/codeminute-the-dev/lattice/coredns-dnsseed/dnsseed" \
    >> plugin.cfg

RUN echo "replace github.com/codeminute-the-dev/lattice => /lattice" >> go.mod

RUN go get github.com/codeminute-the-dev/lattice/coredns-dnsseed/dnsseed && \
    make all && \
    setcap cap_net_bind_service=+ep ./coredns

# ---

FROM alpine:${ALPINE_VERSION}

LABEL maintainer="the Lattice contributors"
LABEL org.opencontainers.image.source="https://github.com/codeminute-the-dev/lattice"
LABEL org.opencontainers.image.description="Lattice DNS Seeder - CoreDNS with dnsseed plugin"

RUN apk --no-cache add bind-tools

COPY --from=builder /coredns/coredns /usr/bin/coredns
COPY --from=builder /etc/ssl/certs /etc/ssl/certs

USER nobody:nobody

EXPOSE 53 53/udp

HEALTHCHECK --interval=5s --retries=10 \
    CMD dig @0.0.0.0 localhost +short +time=1 >/dev/null || exit 1

ENTRYPOINT ["coredns"]
CMD ["-conf", "/etc/coredns/Corefile"]
