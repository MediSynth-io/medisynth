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

# Run the appropriate application based on the SERVICE_TYPE environment variable
CMD ["sh", "-c", "if [ \"$SERVICE_TYPE\" = \"portal\" ]; then if [ \"$MEDISYNTH_ENV\" = \"prod\" ]; then ./medisynth-portal -config app.prod.yml; else ./medisynth-portal -config app.dev.yml; fi; else if [ \"$MEDISYNTH_ENV\" = \"prod\" ]; then ./medisynth-api -config app.prod.yml; else ./medisynth-api -config app.dev.yml; fi; fi"]