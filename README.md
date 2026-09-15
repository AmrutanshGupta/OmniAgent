# OmniAgent

**Production-grade LLM orchestration platform.** OmniAgent decomposes complex user prompts into a Directed Acyclic Graph (DAG) of sub-tasks using a Tree-of-Thoughts planner, then routes each sub-task across multiple LLM tiers (OpenAI, Anthropic, Google, Gemini, Groq) via a dynamic SLA-aware neural router optimizing for cost, latency, quality, and reliability — enforcing a strict budget ledger, transactional state machine, and isolated sandbox critic loop.

---

## Abstract

In typical LLM applications, routing relies on static heuristics — assigning models to task types regardless of real-time conditions. OmniAgent introduces a paradigm shift by treating LLM routing as a live, multi-variable optimization problem continuously reacting to model performance, provider latency, and per-user budget constraints.

At its core, the system does not commit to a model during the planning phase. Instead, a neural router continuously evaluates expected utility across all available model tiers against SLA weights (Quality, Cost, Latency, Reliability) sourced from a live MongoDB-backed registry polling OpenRouter and Artificial Analysis APIs. When a sub-task is ready for execution, the orchestrator makes a just-in-time routing decision.

A deterministic critic loop (no LLM analysis) classifies execution failures into `FailSyntax`, `FailLogic`, or `FailInfrastructure`. Infrastructure failures abort the repair loop immediately; other failures trigger targeted injection of structured error traces into a retry prompt. Budget is atomically deducted after each LLM call, and task dispatch halts entirely if the ledger runs dry.

---

## Architecture Overview

OmniAgent operates as a polyglot system comprising three distinct services and a sandboxed execution engine.

### High-Level System Architecture

```text
+-------------------+       +------------------------+       +-------------------+
|                   |       |                        |       |                   |
|   Next.js Client  |<=====>|    Go Orchestrator     |------>|  Python Backend   |
|   (Dashboard)     |  WS   |   (Task Coordinator)   |  http |  (Planner/Router) |
|                   |       |                        |       |                   |
+-------------------+       +-----------+------------+       +-------------------+
                                        |
                           +------------+------------+
                           |                         |
                           v                         v
               +-----------------------+   +------------------+
               |   Piston API          |   | Ephemeral Docker |
               |   (Script Sandbox)    |   | (Project DooD)   |
               +-----------------------+   +------------------+
```

### Core Components

1. **Go Orchestrator (`backend-go/`)**
   The central nervous system of OmniAgent. Handles concurrent worker execution, manages the transactional DAG state machine with optimistic concurrency control (OCC), performs AES-256-GCM credential decryption in memory, streams real-time events via WebSockets using the Outbox Pattern, enforces per-user budget invariants, and emits structured JSON logs (zerolog) with OpenTelemetry spans at all major boundaries.

2. **Python Backend (`backend-python/`)**
   A FastAPI microservice serving three routers:
   - **`/v1/plan`** — Tree-of-Thoughts planner generating multiple DAG branches with deterministic scoring. Prunes cycles and low-utility branches before selecting the optimal plan.
   - **`/v1/execute-node`** — ReAct-style agent executor backed by LangChain tool-calling. Stores compressed node outputs in MongoDB for upstream context retrieval.
   - **`/v1/router/route`** — SLA-weighted dynamic router computing expected utility scores across live model registry candidates.

3. **Frontend Dashboard (`frontend/`)**
   A Next.js App Router application with a split-pane chat interface. Provides a live DAG visualizer showing node state transitions, tier routing decisions, and compiler feedback in real-time.

4. **Tiered Sandbox Execution**
   - **Piston API** — for lightweight single-file script validation.
   - **Ephemeral Docker (DooD)** — for multi-file projects requiring isolated bridge networks with no external gateways.

---

## Key Architectural Invariants

| Invariant | Enforcement Point |
|-----------|------------------|
| Task dispatch halts if `budget_usd ≤ 0` | `coordinator.go` per-node budget check |
| Budget deducted atomically after every LLM call | `db.DeductBudget` via MongoDB `$inc` |
| State transitions are exclusive and versioned | `coordinator/fsm.go` via OCC + transactions |
| Heartbeat sweeper reclaims expired leases | `coordinator/sweeper.go` background goroutine |
| Outbox events delivered at-least-once | `ws/outbox.go` polling `outbox_events` collection |
| Infrastructure failures abort repair loop | `agents/coder_agent.go` `FailInfrastructure` abort |
| API keys never logged or stored in plaintext | AES-256-GCM JIT decryption in `security/` |
| Fatal `Ping()` blocks before HTTP bind | `cmd/api/main.go` startup sequence |

---

## Execution Flow

```text
  [User Prompt]
        |
        v
 +--------------+
 |  Tree-of-    |----> (3 DAG branch candidates: sequential, parallel, hybrid)
 |  Thoughts    |       scored by completeness × feasibility, cycles pruned
 |  Planner     |
 +------+-------+
        |  best plan selected
        v
 +--------------+      +----------------------------+
 | DAG Builder  |      | Dynamic Model Registry     |
 | (Orchestrator|----->| Live metadata: ELO, cost,  |
 | schedules    |      | latency, context limits,   |
 | nodes via    |      | error rate (EMA-smoothed)  |
 | dep-graph)   |      +-----------+----------------+
 +------+-------+                  |
        |                          | SLA-weighted utility score
        v                          v
 +--------------+      +----------------------------+
 | Task Runner  |<-----| Neural Router              |
 |              |      | Filters: capability,       |
 +------+-------+      | context, budget, privacy   |
        |              +----------------------------+
        v
 +----------------+
 | Sandbox Critic |---(FailSyntax/FailLogic)---> [LLM Repair Loop, max 5 retries]
 | Deterministic  |
 | Classifier     |---(FailInfrastructure)-----> [Abort immediately]
 +------+---------+
        |
     (Passes)
        |
        v
  [Atomic budget deduct → Final Output → Outbox → WebSocket → Client]
```

---

## Setup and Installation

### Prerequisites

- **Docker Desktop** (for MongoDB, Piston sandbox, and DooD execution)
- **Go 1.21+**
- **Node.js 18+**
- **Python 3.12+**

### Quick Setup (Docker Compose)

```bash
# Clone the repository
git clone https://github.com/AmrutanshGupta/OmniAgent.git
cd OmniAgent

# Copy environment templates
cp backend-go/.env.example backend-go/.env
cp frontend/.env.local.example frontend/.env.local

# Edit .env files with your API keys and MongoDB URI
# Then start all services:
docker compose up --build -d
```

### Manual Setup

#### 1. Python Backend
**Folder:** `backend-python/`
```bash
cd backend-python
python -m venv .venv

# Windows
.venv\Scripts\activate
# Linux/macOS
source .venv/bin/activate

pip install -r requirements.txt
uvicorn main:app --host 0.0.0.0 --port 8001 --reload
```

#### 2. Go Orchestrator
**Folder:** `backend-go/`
```bash
cd backend-go
cp .env.example .env
# Edit .env with MONGODB_URI, ENCRYPTION_KEY, NEXTAUTH_SECRET, etc.

go mod tidy
go run cmd/api/main.go
```

#### 3. Frontend Dashboard
**Folder:** `frontend/`
```bash
cd frontend
cp .env.local.example .env.local
npm install
npm run dev
```

#### 4. Diagnostics CLI
Run this anytime to verify all infrastructure is reachable before starting:
```bash
cd backend-go
go run cmd/diagnostic/main.go
```

---

## Environment Variables

### `backend-go/.env`

| Variable | Required | Description |
|----------|----------|-------------|
| `MONGODB_URI` | ✅ | Full MongoDB connection string |
| `MONGODB_NAME` | ✅ | Database name (default: `omniagent`) |
| `ENCRYPTION_KEY` | ✅ | 32-byte AES-256 key (hex-encoded) for BYOK |
| `NEXTAUTH_SECRET` | ✅ | JWT signing secret (must match Next.js) |
| `PORT` | | HTTP port (default: `8080`) |
| `PYTHON_ROUTER_URL` | | Python backend URL (default: `http://localhost:8001`) |
| `ML_ROUTER_URL` | | ML router URL (same as Python backend) |
| `CRON_SCHEDULE` | | Registry refresh cron (default: `@every 6h`) |
| `SANDBOX_KEYWORDS` | | Comma-separated keywords to force sandbox routing |
| `ESCALATION_TIER_ORDER` | | Comma-separated fallback tier order |

---

## Health & Observability

### Health Endpoints (Go Orchestrator)

| Endpoint | Description |
|----------|-------------|
| `GET /health/liveness` | Returns `200 OK` if the HTTP server is up |
| `GET /health/readiness` | Returns `200 OK` only if MongoDB and Registry DB are reachable |

### Structured Logging

All services emit **JSON-structured logs** to stdout:
- **Go:** `rs/zerolog` — fields include `level`, `time`, `message`, and context keys
- **Python:** stdlib `logging` with a custom `JSONFormatter` — fields include `time`, `level`, `name`, `message`

### OpenTelemetry Traces

OTel spans are injected at all major boundaries:
- LLM API calls (`llm_clients.Complete`)
- Neural router calls (`router_client.GetOptimalRoute`)

---

## Security Considerations

- **Bring Your Own Key (BYOK):** API keys are never stored in plaintext. Encrypted using AES-256-GCM before persisting to MongoDB.
- **In-Memory JIT Decryption:** Keys are decrypted only into memory for the duration of an active task execution and never logged.
- **Secret Sanitization:** All log emitters are audited to ensure API key values never appear in any log field.
- **Sandboxed Code Execution:** All generated code runs inside an isolated Docker container (Piston or DooD) on a bridge network with no external gateway.
- **Budget Guardrails:** Per-user `budget_usd` is decremented atomically post-execution. Zero-budget sessions are rejected before any LLM call.

---

## Project Structure

```text
OmniAgent/
├── backend-go/                  # Go Orchestrator
│   ├── cmd/
│   │   ├── api/main.go          # Entrypoint: HTTP/WS server
│   │   └── diagnostic/main.go   # CLI: infrastructure ping tool
│   └── internal/
│       ├── agents/              # coordinator, coder_agent, react_agent
│       ├── config/              # Centralized typed configuration
│       ├── coordinator/         # FSM, optimistic concurrency, lease sweeper
│       ├── critic/              # Deterministic failure classifier & recorder
│       ├── db/                  # MongoDB client, DeductBudget
│       ├── llm_clients/         # Provider clients, circuit breaker
│       ├── registry/            # Dynamic model registry + in-memory cache
│       ├── router_client/       # SLA-weighted routing client
│       ├── sandbox/             # Piston + DooD tiered execution
│       ├── security/            # AES-256-GCM credential service
│       └── ws/                  # WebSocket hub + Outbox poller
│
├── backend-python/              # Python Backend (FastAPI)
│   ├── api/
│   │   ├── planner.py           # Tree-of-Thoughts DAG planner
│   │   └── executor.py          # LangChain ReAct agent executor
│   ├── router/
│   │   └── elo_router.py        # SLA-weighted dynamic router
│   ├── tools/
│   │   └── context_search.py    # MongoDB-backed session context tool
│   └── main.py                  # FastAPI app entrypoint
│
├── frontend/                    # Next.js Dashboard
├── pyrightconfig.json           # Python type-checking config
└── docker-compose.yml           # Full-stack orchestration
```

---

## License

MIT License