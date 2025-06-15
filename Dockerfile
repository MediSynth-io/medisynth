# Use Go 1.23 for both build and runtime
FROM golang:1.23

WORKDIR /app

# Copy only what's needed for go mod download first
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the applications
RUN go build -o medisynth-api ./cmd/api/main.go && \
    go build -o medisynth-portal ./cmd/portal/main.go && \
    chmod +x medisynth-api medisynth-portal

# Create data directory
RUN mkdir -p data

# Install curl for debugging
RUN apt-get update && apt-get install -y curl && rm -rf /var/lib/apt/lists/*

# Expose the application port
EXPOSE 8081

# Default configuration file
ARG CONFIG_FILE=app.yml
ENV CONFIG_FILE=$CONFIG_FILE

# Run the appropriate application based on the SERVICE_TYPE environment variable
CMD if [ "$SERVICE_TYPE" = "portal" ]; then ./medisynth-portal -config $CONFIG_FILE; else ./medisynth-api -config $CONFIG_FILE; fi