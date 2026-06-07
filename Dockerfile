FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o api .


FROM alpine:3.20

WORKDIR /app

COPY --from=builder /app/api .

USER 65534:65534

EXPOSE 8000

ENTRYPOINT ["./api"]

