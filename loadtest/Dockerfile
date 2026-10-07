# Multi-stage build for the Go load tester
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /loadtest .

# --- runtime ---
FROM alpine:3.19

RUN addgroup -S app && adduser -S app -G app
COPY --from=builder /loadtest /loadtest

USER app
ENTRYPOINT ["/loadtest"]
