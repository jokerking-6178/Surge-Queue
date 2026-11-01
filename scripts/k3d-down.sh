#!/bin/bash
set -e

echo "=== Deleting k3d cluster ==="
k3d cluster delete surge-queue

echo "=== Removing local images ==="
docker rmi queue-engine:local backend:local frontend:local 2>/dev/null || echo "Some images may not exist"

echo "=== Cleanup complete ==="
