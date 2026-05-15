# syntax=docker/dockerfile:1.4

# Builder stage
FROM golang:1.26 AS builder

ARG VERSION=dev
ARG COMMIT=dev
ARG BUILDDATE=dev

WORKDIR /src

# Cache dependency downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build binaries (pure Go, no CGO needed — uses modernc.org/sqlite)
RUN CGO_ENABLED=0 go build \
      -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildDate=${BUILDDATE}" \
      -trimpath \
      -o /go/bin/ \
      ./cmd/vassago \
      ./daemon/cmd/vassago-daemon \
      ./daemon/cmd/vassago-cli \
      ./daemon/cmd/vassago-import

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /go/bin/vassago /usr/local/bin/vassago
COPY --from=builder /go/bin/vassago-daemon /usr/local/bin/vassago-daemon
COPY --from=builder /go/bin/vassago-cli /usr/local/bin/vassago-cli
COPY --from=builder /go/bin/vassago-import /usr/local/bin/vassago-import

EXPOSE 50051

VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["vassago", "health"]

ENTRYPOINT ["vassago"]
CMD ["daemon"]
