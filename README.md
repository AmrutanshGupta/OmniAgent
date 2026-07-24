# OmniAgent — Infrastructure-Aware LLM Orchestrator

A polyglot Coordinator-Worker LLM orchestrator implementing:
- **Planner Agent** — Tree-of-Thoughts (ToT): evaluates 3 architectural decompositions (sequential, parallel-specialist, hierarchical), scores them on scalability/cost, executes the winning DAG.
- **Infra-Aware Router Agent** — a PyTorch `SharedEncoderNet` (FastAPI service) that picks a model tier (`flash` / `sonnet` / `gpt-4o`) from prompt complexity + live queue depth + your SLA slider.
- **Domain-Specialist Workers** — Go goroutines that execute each DAG task at the tier the router assigned.
- **Critic Agent** — FrugalGPT cascade: validates each worker's output and automatically escalates to a smarter tier on failure.
- **Synthesizer Agent** — pure Go, zero LLM calls, merges completed task outputs into the final answer.

Everything streams live to the React/Next.js UI over WebSockets (DAG visualizer, routing HUD, cost tracker).

**Runs with zero API keys** in simulated mode so you can see the whole pipeline work end-to-end immediately. Add your own OpenAI/Anthropic/Google keys in Settings for real completions (BYOK, AES-256-GCM encrypted at rest).

---

## 1. Project layout

```
llm-orchestrator/
├── backend-go/     # Go/Fiber orchestrator (agents, WebSocket hub, BYOK vault)
├── router-ml/       # Python/FastAPI + PyTorch adaptive router
├── frontend/        # Next.js UI (chat + live DAG + telemetry)
└── docker-compose.yml
```

---

## 2. Option A — Run everything with Docker (easiest)

**Prerequisites:** Docker + Docker Compose installed.

```bash
cd llm-orchestrator
docker compose up --build
```

Then open:
- **Frontend:** http://localhost:3000
- **Go API:** http://localhost:8080/api/health
- **Router ML API:** http://localhost:8000/health

That's it — the three services start and talk to each other automatically. Ctrl+C to stop, `docker compose up --build` again after code changes.

---

## 3. Option B — Run each service manually (for development)

You'll need **3 terminals**, and these installed locally:
- Go **1.21+**
- Python **3.11+**
- Node.js **20+** and npm

### Terminal 1 — Router ML (Python/FastAPI/PyTorch)

```bash
cd llm-orchestrator/router-ml
python3 -m venv venv
source venv/bin/activate        # Windows: venv\Scripts\activate
pip install -r requirements.txt
uvicorn app.main:app --reload --port 8000
```
First boot writes a seeded PyTorch checkpoint to `weights/adaptive_router_v1.pt` automatically.
Check it's alive: http://localhost:8000/health

### Terminal 2 — Go Orchestrator (Fiber)

```bash
cd llm-orchestrator/backend-go
go mod tidy          # downloads fiber, websocket, uuid
go run ./cmd/api
```
Check it's alive: http://localhost:8080/api/health

By default it looks for the router at `http://localhost:8000` — override with `ROUTER_ML_URL` if needed (see `.env.example`).

### Terminal 3 — Frontend (Next.js)

```bash
cd llm-orchestrator/frontend
npm install
npm run dev
```
Open http://localhost:3000

---

## 4. Using it

1. Open http://localhost:3000 — you'll see a split-pane workspace: **Chat** on the left, **live DAG + telemetry** on the right.
2. Type a prompt, e.g.:
   > "Build a REST API endpoint that analyzes CSV upload data and writes a short marketing summary of the results."
3. Watch the right pane:
   - The **DAG visualizer** renders the winning ToT plan's tasks as nodes (grey → blue → green, or red if escalated).
   - The **Routing HUD** shows which tier (flash/sonnet/gpt-4o) is active.
   - The **Cost Tracker** shows spend and FrugalGPT savings vs always using the top-tier model.
4. Go to **Settings → API Vault** to add your own OpenAI / Anthropic / Google API keys (stored in your browser, sent per-request, encrypted at rest server-side with AES-256-GCM, never logged). Leave them blank to keep running in simulated mode.
5. Use the **Save Money ↔ Max Quality slider** above the chat box to bias the router toward cheaper or smarter tiers.

---

## 5. How the pipeline actually works, end to end

1. **POST /api/plan** hits the Go backend with `{ prompt, sla, keys }`.
2. `PlanToT()` builds 3 candidate DAGs (sequential / parallel-specialist / hierarchical), scores each on `scalability*0.5 + cost*0.5`, picks the winner. This plan is broadcast over WebSocket immediately (`plan_ready`) so the UI can draw the graph before any task runs.
3. The **Coordinator** (`RunPipeline`) walks the DAG in dependency "waves" — all tasks whose dependencies are done run concurrently as goroutines.
4. Each task first asks **router-ml** (`POST /route`) for a tier, based on prompt complexity, current queue depth, and your quality/cost slider. If router-ml is unreachable, the Go side falls back to a local heuristic so the app never hard-fails.
5. The **Worker** calls the assigned tier's LLM (real API call if you supplied a BYOK key, otherwise a simulated deterministic response) and streams `task_update` events as it goes.
6. The **Critic** checks the output (empty/error/too-short/malformed-code heuristics). If it fails, `Escalate()` bumps the task to the next rung of its FrugalGPT cascade (`flash → sonnet → gpt-4o`) and it re-runs — this is the "reject and escalate" loop from the PRD.
7. Once every task in the DAG is `completed`, the **Synthesizer** (pure Go, no LLM calls) merges all outputs into the final markdown-style answer, and cost telemetry (`run_complete`) is broadcast.

---

## 6. Security notes

- BYOK keys are never written to server logs (Fiber's logger middleware format excludes bodies).
- `POST /api/vault/encrypt` demonstrates AES-256-GCM at-rest encryption (key derived from `ORCH_MASTER_SECRET` env var — **change this in production**, it defaults to an insecure dev value).
- Decrypted keys only ever live in-memory for the duration of a single request/goroutine — they're never persisted server-side in this reference implementation (wire up MongoDB in `backend-go/internal/security` if you want persistent BYOK storage across sessions).

## 7. Notes on the "zero-cost serverless" deployment target

This repo is structured so each piece deploys independently exactly as described in the PRD:
- `frontend/` → Vercel (Next.js, zero config)
- `backend-go/` → Render/Koyeb (Dockerfile included, multi-stage build)
- `router-ml/` → Hugging Face Spaces (Dockerfile included, CPU-only)

For production you'd add MongoDB Atlas (task/user persistence) and Upstash Redis (queue depth telemetry) — the current reference implementation uses in-memory state so it runs completely free with **zero external services** for local development, per the "runs when downloaded" requirement.

## 8. Troubleshooting

| Symptom | Fix |
|---|---|
| Frontend shows "○ reconnecting…" | Go backend isn't running / wrong `NEXT_PUBLIC_GO_API_URL` in `frontend/.env.local` |
| Router calls fall back to heuristic tier | `router-ml` isn't running / wrong `ROUTER_ML_URL` |
| All outputs say "[SIMULATED ...]" | Expected with no BYOK keys — add keys in Settings for real completions |
| `go: command not found` | Install Go 1.21+ from https://go.dev/dl/ |
| `pip install torch` slow/fails | Use Python 3.11 (not 3.13) and ensure you have ~2GB free disk space |
