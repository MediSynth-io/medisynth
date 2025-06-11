FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o medisynth-api ./cmd/api/main.go

FROM alpine:latest

# Install OpenJDK 11 (a common choice for Synthea) and curl for healthcheck
RUN apk add --no-cache openjdk11-jre curl

# Create a directory for the application and Synthea
RUN mkdir -p /app
WORKDIR /app

# Download Synthea JAR.
# IMPORTANT: Check Synthea GitHub releases for the latest stable version and update the URL.
# Using a specific version is highly recommended for reproducibility.
# Example using Synthea v3.3.0 (check if this is still current/desired):
ARG SYNTHEA_VERSION=3.3.0
RUN wget "https://github.com/synthetichealth/synthea/releases/download/v${SYNTHEA_VERSION}/synthea-with-dependencies.jar" -O /app/synthea-with-dependencies.jar

# Alternatively, if you download the JAR manually and place it in the build context (e.g., medisynth/synthea.jar):
# COPY synthea.jar /app/synthea-with-dependencies.jar

# Create non-root user and group
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Copy app.yml and set ownership
COPY app.yml /app/app.yml
RUN chown appuser:appgroup /app/app.yml

# Copy the built Go application and set ownership
COPY --from=builder --chown=appuser:appgroup /app/medisynth-api /app/medisynth-api

# Switch to non-root user
USER appuser
# WORKDIR is already /app, which is where app.yml and medisynth-api are.

EXPOSE 8080

# Healthcheck (increased start-period slightly for Java initialization)
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD curl -f http://localhost:8080/heartbeat || exit 1

CMD ["./medisynth-api"]