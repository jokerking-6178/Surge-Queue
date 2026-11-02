#!/bin/bash
set -e

# Change to script directory
cd "$(dirname "$0")/.."

echo "=== Building Docker images ==="
docker build -t queue-engine:local ./queue-engine
docker build -t backend:local ./backend
docker build -t frontend:local ./frontend

echo "=== Creating k3d cluster ==="
k3d cluster create surge-queue --port "8080:80@loadbalancer" --port "8081:80@loadbalancer" --port "3000:80@loadbalancer" --agents 0 || echo "Cluster may already exist"

echo "=== Loading images into cluster ==="
k3d image import queue-engine:local -c surge-queue
k3d image import backend:local -c surge-queue
k3d image import frontend:local -c surge-queue

echo "=== Creating JWT secret ==="
if [ -z "$JWT_SECRET" ]; then
  echo "Error: JWT_SECRET environment variable not set"
  echo "Create a .env file with JWT_SECRET or export it"
  exit 1
fi
kubectl create secret generic jwt-secret -n surge-system --from-literal=JWT_SECRET="$JWT_SECRET" --dry-run=client -o yaml | kubectl apply -f -

echo "=== Applying Kubernetes manifests ==="
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/redis.yaml
kubectl apply -f k8s/queue-engine.yaml
kubectl apply -f k8s/backend.yaml
kubectl apply -f k8s/frontend.yaml

echo "=== Waiting for deployments to be ready ==="
kubectl wait --for=condition=available -n surge-system deployment/redis --timeout=60s
kubectl wait --for=condition=available -n surge-system deployment/queue-engine --timeout=60s
kubectl wait --for=condition=available -n surge-system deployment/backend --timeout=120s
kubectl wait --for=condition=available -n surge-system deployment/frontend --timeout=60s

echo ""
echo "=== Setup complete! ==="
echo "Open your browser to: http://localhost:3000"
echo ""
echo "To check pod status:"
echo "  kubectl get pods -n surge-system"
echo ""
echo "To view logs:"
echo "  kubectl logs -n surge-system deployment/frontend"
echo "  kubectl logs -n surge-system deployment/queue-engine"
echo "  kubectl logs -n surge-system deployment/backend"
