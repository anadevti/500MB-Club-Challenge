# Build stage
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w" \
    -trimpath \
    -o /api ./cmd/api

# Runtime stage
FROM scratch

COPY --from=builder /api /api

EXPOSE 8000

ENTRYPOINT ["/api"]
