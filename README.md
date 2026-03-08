# Network-Aware Kubernetes Autoscaler

A custom Kubernetes controller written in Go that scales deployments based on **CPU utilization**, **memory usage**, and **p95 latency** metrics collected from Prometheus.

Unlike the default Horizontal Pod Autoscaler (HPA) which only supports CPU/memory, this controller adds network-aware scaling by incorporating p95 request latency — making it suitable for latency-sensitive services.

## Architecture

```
┌─────────────────────────────────────────────────┐
│              Autoscaler Controller               │
│                                                 │
│  ┌─────────────┐      ┌──────────────────────┐  │
│  │  Prometheus  │ ───► │   Metrics Snapshot   │  │
│  │   Client     │      │  CPU / Mem / p95     │  │
│  └─────────────┘      └──────────┬───────────┘  │
│                                  │               │
│                        ┌─────────▼────────┐      │
│                        │  Scaler Engine   │      │
│                        │  (Evaluate())    │      │
│                        └─────────┬────────┘      │
│                                  │               │
│                        ┌─────────▼────────┐      │
│                        │  K8s Deployment  │      │
│                        │  Update (scale)  │      │
│                        └──────────────────┘      │
└─────────────────────────────────────────────────┘
```

## Scaling Logic

| Condition | Action |
|-----------|--------|
| CPU > 75% | Scale up by 2 replicas |
| Memory > 80% | Scale up by 2 replicas |
| p95 latency > 200ms | Scale up by 2 replicas |
| All metrics < 50% of threshold | Scale down by 1 replica |
| Otherwise | No action |

- **Max replicas:** 10
- **Min replicas:** 1
- Scale-down is conservative — requires ALL metrics below 50% of their thresholds

## Prerequisites

- Kubernetes 1.22+
- Prometheus with `kube-state-metrics` and `metrics-server`
- Go 1.17+

## Running Locally (minikube)

```bash
# Start minikube
minikube start

# Install Prometheus stack
kubectl apply -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/main/bundle.yaml

# Deploy autoscaler
kubectl apply -f deploy/autoscaler.yaml

# Label a deployment to enable autoscaling
kubectl label deployment my-app network-autoscaler=enabled

# Run locally (out-of-cluster)
go run cmd/controller/main.go \
  --kubeconfig=$HOME/.kube/config \
  --prometheus-url=http://localhost:9090 \
  --sync-interval=30s \
  --cpu-threshold=75 \
  --mem-threshold=80 \
  --p95-threshold=200
```

## Project Structure

```
.
├── cmd/
│   └── controller/
│       └── main.go          # Entry point, CLI flags
├── pkg/
│   ├── controller/
│   │   └── autoscaler.go    # Control loop, deployment reconciliation
│   ├── metrics/
│   │   └── prometheus.go    # Prometheus PromQL queries
│   └── scaler/
│       └── scaler.go        # Scaling decision logic
├── deploy/
│   └── autoscaler.yaml      # K8s manifests (Deployment, RBAC)
└── go.mod
```

## Key Design Decisions

- **client-go** used directly instead of controller-runtime for learning purposes and fine-grained control
- **p95 latency** sourced from `http_request_duration_seconds` histogram — requires instrumented services
- Scale-up is aggressive (any single metric breach triggers it); scale-down is conservative (all metrics must be low)
- Deployments opt-in via label `network-autoscaler=enabled`

## Tech Stack

- **Go 1.17**
- **client-go v0.22** — Kubernetes API client
- **Prometheus HTTP API** — metrics source
- **minikube** — local cluster for development/testing
