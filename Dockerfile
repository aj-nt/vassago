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
      ./cmd/mnemosyne \
      ./daemon/cmd/mnemosyne-daemon \
      ./daemon/cmd/mnemosyne-cli \
      ./daemon/cmd/mnemosyne-import

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /go/bin/mnemosyne /usr/local/bin/mnemosyne
COPY --from=builder /go/bin/mnemosyne-daemon /usr/local/bin/mnemosyne-daemon
COPY --from=builder /go/bin/mnemosyne-cli /usr/local/bin/mnemosyne-cli
COPY --from=builder /go/bin/mnemosyne-import /usr/local/bin/mnemosyne-import

EXPOSE 50051

VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["mnemosyne", "health"]

ENTRYPOINT ["mnemosyne"]
CMD ["daemon"]
