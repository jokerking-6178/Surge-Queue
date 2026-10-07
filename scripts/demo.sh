#!/usr/bin/env bash
# demo.sh — runs the full surge queue demo: with-queue vs without-queue.
#
# Shows how the queue keeps the backend smooth under burst load,
# vs what happens when traffic hits the backend directly.
#
set -euo pipefail

BOLD="\033[1m"
GREEN="\033[32m"
RED="\033[31m"
YELLOW="\033[33m"
RESET="\033[0m"

echo -e "${BOLD}╔══════════════════════════════════════════════════════╗${RESET}"
echo -e "${BOLD}║   Surge Protection Queue System — Live Demo           ║${RESET}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════╝${RESET}"
echo ""

# --- Start services ---
echo -e "${YELLOW}Starting services (Redis + Queue Engine + Backend)...${RESET}"
docker compose up -d --build --wait
echo -e "${GREEN}Services are up!${RESET}"
echo ""

# --- Test 1: Without Queue (chaos) ---
echo -e "${BOLD}═══ Test 1: WITHOUT queue (direct hit on backend) ═══${RESET}"
echo -e "${YELLOW}Simulating 500 users hitting the backend simultaneously...${RESET}"
echo ""
cd loadtest
go run . \
  --mode=without-queue \
  --users=500 \
  --ramp=1s \
  --backend=http://localhost:8082
echo ""
echo -e "${RED}↑ Notice the 5xx errors and failures — the backend couldn't handle the burst.${RESET}"
echo ""

# --- Test 2: With Queue (smooth) ---
echo -e "${BOLD}═══ Test 2: WITH queue (protected) ═══${RESET}"
echo -e "${YELLOW}Same 500 users, but now going through the queue engine...${RESET}"
echo ""
go run . \
  --mode=with-queue \
  --users=500 \
  --ramp=1s \
  --queue=http://localhost:8080 \
  --backend=http://localhost:8081
echo ""
echo -e "${GREEN}↑ Every user processed smoothly — zero errors, orderly admission.${RESET}"
echo ""

# --- Cleanup ---
echo -e "${YELLOW}Stopping services...${RESET}"
cd ..
docker compose down
echo -e "${GREEN}Done!${RESET}"
