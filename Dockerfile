# Stage 1: Build binary
FROM golang:alpine AS builder

ENV GOTOOLCHAIN=auto

WORKDIR /app

# Copy all source files
COPY . .

# Download dependencies and build static binary
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o earnsmart-server ./cmd/server/main.go

# Stage 2: Minimal runtime image
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/earnsmart-server /app/earnsmart-server
COPY --from=builder /app/schema.sql /app/schema.sql
COPY --from=builder /app/migrations /app/migrations

EXPOSE 8080

CMD ["/app/earnsmart-server"]
