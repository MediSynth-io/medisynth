# Build stage
FROM golang:1.23 as builder

# Install build dependencies
RUN apt-get update && apt-get install -y gcc

WORKDIR /app

# Download all dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the application with CGO enabled
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o medisynth-api ./cmd/api/main.go

# Use a smaller image for the final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/medisynth-api .

# Expose the application port
EXPOSE 8081

# Run the application
CMD ["./medisynth-api"] 