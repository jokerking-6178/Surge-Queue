#!/bin/bash
set -e

# Change to script directory
cd "$(dirname "$0")/.."

REBUILD=false
if [ "$1" = "--rebuild" ]; then
  REBUILD=true
fi

echo "=== Checking k3d cluster ==="
if k3d cluster get surge-queue &>/dev/null; then
  echo "Cluster 'surge-queue' exists"
  if k3d cluster list | grep -q "surge-queue.*running"; then
    echo "Cluster is running"
  else
    echo "Starting stopped cluster..."
    k3d cluster start surge-queue
  fi
else
  echo "Creating new cluster..."
  k3d cluster create surge-queue --port "8080:80@loadbalancer" --port "8081:80@loadbalancer" --port "3000:80@loadbalancer" --agents 0
fi

# Build images if --rebuild or if they don't exist
if [ "$REBUILD" = true ] || ! docker image inspect queue-engine:local &>/dev/null; then
  echo "=== Building Docker images ==="
  docker build -t queue-engine:local ./queue-engine
  docker build -t backend:local ./backend
  docker build -t frontend:local ./frontend

  echo "=== Loading images into cluster ==="
  k3d image import queue-engine:local -c surge-queue
  k3d image import backend:local -c surge-queue
  k3d image import frontend:local -c surge-queue
else
  echo "=== Docker images already exist (use --rebuild to rebuild) ==="
fi

echo "=== Creating JWT secret ==="
if [ -z "$JWT_SECRET" ]; then
  echo "Error: JWT_SECRET environment variable not set"
  echo "Create a .env file with JWT_SECRET or export it"
  exit 1
fi
kubectl create secret generic jwt-secret -n surge-system --from-literal=JWT_SECRET="$JWT_SECRET" --dry-run=client -o yaml | kubectl apply -f -

echo "=== Applying Kubernetes manifests ==="
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/

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
