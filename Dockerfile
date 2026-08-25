# APCode Dockerfile — like opencode's `docker run ghcr.io/anomalyco/opencode`
# Build: docker build -t apcode .
# Run:   docker run --rm -it apcode --help
#        docker run --rm -it -v $(pwd):/work apcode recommend
#        docker run --rm -it -v $(pwd):/work apcode search "func main"

ARG VERSION=0.1.0
FROM golang:1.26-alpine AS builder
ARG VERSION
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X apcode/internal/config.Version=${VERSION}" -o /out/apcode ./cmd/apcode

FROM alpine:3.20
RUN apk add --no-cache ca-certificates git
COPY --from=builder /out/apcode /usr/local/bin/apcode
WORKDIR /work
ENTRYPOINT ["apcode"]
CMD ["--help"]
LABEL org.opencontainers.image.source="https://github.com/anshulchikhale30-p/APCode"
LABEL org.opencontainers.image.description="APCode — offline-first AI coding agent"
LABEL org.opencontainers.image.licenses="MIT"
