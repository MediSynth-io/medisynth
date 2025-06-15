#!/bin/bash

# Create data directory if it doesn't exist
mkdir -p data

# Create self-signed certificates for development
mkdir -p certs
if [ ! -f certs/medisynth.key ]; then
    openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
        -keyout certs/medisynth.key \
        -out certs/medisynth.crt \
        -subj "/C=US/ST=State/L=City/O=MediSynth/CN=*.medisynth.io"
fi

# Add local domain entries to /etc/hosts if they don't exist
if ! grep -q "portal.local" /etc/hosts; then
    echo "127.0.0.1 portal.local" | sudo tee -a /etc/hosts
fi

if ! grep -q "api.local" /etc/hosts; then
    echo "127.0.0.1 api.local" | sudo tee -a /etc/hosts
fi

# Create development secrets
kubectl create namespace medisynth-io --dry-run=client -o yaml | kubectl apply -f -

# Create development secrets
kubectl create secret generic medisynth-secrets \
    --namespace medisynth-io \
    --from-literal=jwt-secret=dev_jwt_secret \
    --from-literal=session-secret=dev_session_secret \
    --dry-run=client -o yaml | kubectl apply -f -

# Apply development configuration
kubectl apply -k .devops/k8s/overlays/dev

echo "Development environment is ready!"
echo "Access the portal at: http://portal.local"
echo "Access the API at: http://api.local" 