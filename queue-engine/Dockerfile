# Multi-stage build for the Go queue engine
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Cache deps first
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /queue-engine .

# --- runtime ---
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /queue-engine /queue-engine

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/queue-engine"]
