# Surge Protection Queue System

A production-grade virtual queue system that sits in front of a website/app and manages heavy traffic spikes — exam counselling, event tickets, college fest sign-ups, IRCTC-style booking. Instead of letting everyone hit the server at once and crashing it, users enter a live queue, see their position, and get admitted smoothly and in order.

Built the way real companies (Ticketmaster, SeatGeek, IRCTC) run dedicated queue systems for high-demand sales — because regular cloud auto-scaling is too slow to react to a sudden spike in the first few seconds.

## Architecture

```
                    ┌─────────────┐
   Users ──────────►│   Ingress   │  (TLS, rate-limit, nginx)
                    │  (OCI LB)   │
                    └──────┬──────┘
                           │
              ┌────────────┼────────────┐
              ▼                         ▼
    ┌──────────────────┐      ┌──────────────────┐
    │  Queue Engine    │      │     Backend      │
    │     (Go)         │      │  (Spring Boot)   │
    │                  │      │                  │
    │ • Enter queue    │      │ • /api/register   │
    │ • Check position │      │ • JWT validation  │
    │ • Heartbeat      │      │ • Registration    │
    │ • Admit (JWT)    │      │   logic           │
    │                  │      │                  │
    │ HPA: 2→8 pods   │      │ HPA: 3→10 pods   │
    └────────┬─────────┘      └──────────────────┘
             │
             ▼
    ┌──────────────┐
    │    Redis     │  (OCI Cache / managed)
    │  (backing    │
    │   store)     │
    └──────────────┘
```

### How it works

1. **User enters the queue** — `POST /api/queue/enter` adds them to a Redis sorted set (ZSET), scored by timestamp (FIFO ordering). Returns their position and estimated wait.

2. **User polls status** — `GET /api/queue/status/{userId}` returns current position, total waiting, and ETA. The frontend displays a live "you are #42 in line" UI.

3. **Admitter goroutine** runs in the background, admitting users at a configurable rate (`ADMIT_RATE_PER_SEC`, default 25/sec) up to `MAX_CONCURRENT_ADMITTED` (default 200). When a user is admitted, they're removed from the queue and issued a **signed JWT admit token**.

4. **User hits the backend** — `POST /api/register` with the `Authorization: Bearer <admit-token>` header. The backend's `QueueTokenFilter` validates the JWT (same secret, injected via K8s secrets / OCI Vault). No token = 401.

5. **Session expiry** — admitted sessions auto-expire after `ADMITTED_SESSION_TTL` (default 300s), freeing slots for the next users in line.

6. **Stale entry cleanup** — users who stop polling (tab closed) are evicted after `WAITING_TTL` (default 600s).

## Tech Stack

| Component | Technology | Purpose |
|-----------|-----------|---------|
| Queue Engine | Go 1.22, chi, go-redis | Handles thousands of status checks concurrently |
| Backend | Java 21, Spring Boot 3.2 | Registration/booking logic |
| Backing Store | Redis 7 (OCI Cache) | Sorted-set queue + session tracking |
| Infrastructure | Terraform, OCI (OKE) | VCN, OKE cluster, OCI Cache, Container Registry, KMS Vault |
| Orchestration | Kubernetes, Docker | Deployments, HPA, NetworkPolicy, Ingress |
| Monitoring | Prometheus, Grafana | Queue depth, admission rate, latency dashboards |
| TLS | cert-manager, Let's Encrypt | Automatic HTTPS |
| Secrets | OCI Vault + ExternalSecrets | JWT secret synced into K8s Secret |
| CI/CD | GitHub Actions | Build, push, terraform, deploy |

## Quick Start (Local Demo)

### Prerequisites
- Docker + Docker Compose
- Go 1.22+ (for the load tester)

### Run the demo

```bash
# 1. Start all services
docker compose up --build

# 2. In another terminal, run the demo script
chmod +x scripts/demo.sh
./scripts/demo.sh
```

The demo script runs two tests back-to-back:

**Test 1 — Without queue (chaos):** 500 users slam the backend directly. You'll see 5xx errors and failures — the backend can't handle the burst.

**Test 2 — With queue (protected):** The same 500 users go through the queue engine. Every user is processed smoothly with zero errors, admitted at a controlled rate.

### Manual testing

```bash
# Start services
docker compose up --build

# Enter the queue
curl -X POST http://localhost:8080/api/queue/enter \
  -H "Content-Type: application/json" \
  -d '{"user_id":"user-1"}'

# Check status
curl http://localhost:8080/api/queue/status/user-1

# Once admitted (state: "admitted"), register
curl -X POST http://localhost:8081/api/register \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <admit_token>" \
  -d '{"name":"Test User","email":"test@example.com"}'

# Check stats
curl http://localhost:8081/api/stats
```

### Load testing

```bash
cd loadtest

# With queue (protected)
go run . --mode=with-queue --users=2000 --ramp=3s \
  --queue=http://localhost:8080 \
  --backend=http://localhost:8081

# Without queue (unprotected)
go run . --mode=without-queue --users=2000 --ramp=3s \
  --backend=http://localhost:8082  # requires: docker compose --profile demo up
```

## API Reference

### Queue Engine (port 8080)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/queue/enter` | POST | Enter the queue. Body: `{"user_id":"optional"}` |
| `/api/queue/status/{userId}` | GET | Get position, ETA, and admit token (if admitted) |
| `/api/queue/heartbeat/{userId}` | POST | Refresh waiting TTL |
| `/healthz` | GET | Liveness probe |
| `/readyz` | GET | Readiness probe |
| `/metrics` | GET | Prometheus metrics |

**Response (waiting):**
```json
{
  "user_id": "user-1",
  "status": {
    "state": "waiting",
    "position": 42,
    "total_waiting": 500,
    "eta_seconds": 100
  }
}
```

**Response (admitted):**
```json
{
  "user_id": "user-1",
  "status": {
    "state": "admitted",
    "admit_token": "eyJhbGciOiJIUzI1NiIs...",
    "total_waiting": 499
  }
}
```

### Backend (port 8081)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/register` | POST | Submit registration (requires admit token) |
| `/api/stats` | GET | Get total registration count |
| `/api/health` | GET | Health check |
| `/actuator/health` | GET | Spring Boot health |
| `/actuator/prometheus` | GET | Prometheus metrics |

## Configuration

### Queue Engine (environment variables)

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_PORT` | 8080 | HTTP listen port |
| `REDIS_ADDR` | localhost:6379 | Redis address |
| `ADMIT_RATE_PER_SEC` | 25 | Users admitted per second |
| `MAX_CONCURRENT_ADMITTED` | 200 | Max active (admitted) sessions |
| `ADMITTED_SESSION_TTL` | 300 | Seconds before an admitted session expires |
| `WAITING_TTL` | 600 | Seconds before a waiting entry is evicted (no heartbeat) |
| `JWT_SECRET` | dev-secret-change-me | Secret for signing/validating admit tokens |
| `JWT_TTL` | 600 | Admit token validity (seconds) |

### Backend (environment variables)

| Variable | Default | Description |
|----------|---------|-------------|
| `JWT_SECRET` | dev-secret-change-me | Must match queue engine |
| `ENFORCE_QUEUE_TOKEN` | true | Set to false for "without queue" demo mode |

## Deployment to OCI (Oracle Cloud)

### Prerequisites
1. OCI account with access to OKE, OCI Cache, Container Registry, and KMS Vault
2. API key configured in `~/.oci/config`
3. A compartment for resources

### Steps

```bash
cd infra

# 1. Copy and fill in tfvars
cp environments/dev/terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your OCIDs

# 2. Init
terraform init

# 3. Plan
terraform plan

# 4. Apply
terraform apply

# 5. Get kubeconfig
oci ce cluster create-kubeconfig \
  --cluster-id <cluster_ocid> \
  --file ~/.kube/config \
  --region ap-mumbai-1

# 6. Install cert-manager
helm install cert-manager jetstack/cert-manager \
  -n cert-manager --create-namespace --set installCRDs=true

# 7. Install External Secrets Operator
helm install external-secrets external-secrets/external-secrets \
  -n external-secrets --create-namespace

# 8. Install monitoring stack
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
  -n surge-monitoring --create-namespace \
  -f deploy/monitoring/values.yaml

# 9. Deploy the application
kubectl apply -k deploy/base/

# 10. Check rollout
kubectl -n surge-system rollout status deployment/queue-engine
kubectl -n surge-system rollout status deployment/backend
```

## Project Structure

```
surge-queue/
├── queue-engine/          # Go queue engine (the core)
│   ├── main.go            # Entry point, HTTP server, graceful shutdown
│   ├── api/handlers.go    # HTTP route handlers
│   ├── config/config.go   # Environment configuration
│   ├── jwt/jwt.go         # JWT signing/verification
│   ├── metrics/metrics.go # Prometheus metrics
│   ├── queue/queue.go     # Redis-backed queue logic + admitter
│   ├── Dockerfile
│   └── go.mod
├── backend/               # Java Spring Boot backend
│   ├── src/main/java/com/surge/backend/
│   │   ├── BackendApplication.java
│   │   ├── config/        # Security properties, filter registration
│   │   ├── security/       # QueueTokenFilter (JWT validation)
│   │   ├── controller/     # RegistrationController
│   │   ├── service/        # RegistrationService
│   │   └── model/          # RegistrationRequest
│   ├── src/main/resources/application.yml
│   ├── Dockerfile
│   └── pom.xml
├── loadtest/              # Go load tester (with-queue vs without-queue)
│   ├── main.go
│   └── Dockerfile
├── infra/                 # Terraform for OCI
│   ├── modules/
│   │   ├── vcn/            # VCN, subnets, gateways, security lists
│   │   ├── oke/            # OKE cluster + node pool
│   │   ├── redis/          # OCI Cache (managed Redis)
│   │   └── ocr/            # OCI Container Registry repos
│   ├── main.tf
│   ├── variables.tf
│   ├── versions.tf
│   └── environments/dev/
├── deploy/                # Kubernetes manifests
│   ├── base/
│   │   ├── namespace.yaml
│   │   ├── secret-externalsecret.yaml
│   │   ├── redis.yaml
│   │   ├── queue-engine.yaml    # Deployment + Service + HPA
│   │   ├── backend.yaml         # Deployment + Service + HPA
│   │   ├── networkpolicy.yaml
│   │   ├── ingress.yaml         # TLS via cert-manager
│   │   └── kustomization.yaml
│   └── monitoring/
│       ├── servicemonitors.yaml
│       ├── values.yaml          # kube-prometheus-stack Helm values
│       └── grafana-dashboard.yaml
├── .github/workflows/
│   └── ci-cd.yaml          # Build, push, terraform, deploy
├── scripts/
│   └── demo.sh             # One-command demo
├── docker-compose.yml      # Local dev environment
└── README.md
```

## Key Design Decisions

**Why a sorted set (ZSET) in Redis?** O(log N) insertion and removal, natural FIFO ordering by timestamp, and atomic `ZPOPMIN` for safe concurrent admission. Redis is battle-tested for this — it's the same pattern used by real queue systems.

**Why JWT for admit tokens?** Stateless — the backend doesn't need to call back to the queue engine to verify admission. The shared secret means the backend can independently validate tokens, reducing latency and coupling.

**Why a background admitter goroutine?** Decouples admission rate from request rate. Even if 10,000 users enter the queue in 1 second, the admitter only processes `ADMIT_RATE_PER_SEC` per tick, protecting the backend. This is the core insight: rate-limit admission, not just requests.

**Why NetworkPolicy?** Defense in depth. Even if someone bypasses the queue, the NetworkPolicy ensures only the ingress controller and queue-engine pods can reach the backend. The JWT check is the application-layer gate; NetworkPolicy is the network-layer gate.

**Why HPA on queue depth?** CPU-based autoscaling reacts to load that's already hitting pods. Queue-depth-based autoscaling anticipates — if the queue is growing, scale up before the existing pods are overwhelmed.

## License

MIT — see the project for details. Built for educational and production use.
