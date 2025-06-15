#!/bin/bash

# Check if production secrets are set
if [ -z "$MEDISYNTH_JWT_SECRET" ] || [ -z "$MEDISYNTH_SESSION_SECRET" ]; then
    echo "Error: Production secrets not set"
    echo "Please set MEDISYNTH_JWT_SECRET and MEDISYNTH_SESSION_SECRET"
    exit 1
fi

# Check if SSL certificates exist
if [ ! -f certs/medisynth.crt ] || [ ! -f certs/medisynth.key ]; then
    echo "Error: SSL certificates not found in certs/ directory"
    echo "Please place your SSL certificates as certs/medisynth.crt and certs/medisynth.key"
    exit 1
fi

# Set production environment
export MEDISYNTH_ENV=prod

# Stop any running containers
docker-compose down

# Build and start the production environment
docker-compose build
docker-compose up -d

echo "Production environment is deployed!"
echo "Access the portal at: https://portal.medisynth.io"
echo "Access the API at: https://api.medisynth.io" 