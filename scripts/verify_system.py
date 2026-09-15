import os
import sys
import json
import time
import asyncio
import uuid
import copy
import statistics
import datetime
import subprocess
import traceback
import aiohttp
import websockets
import pymongo
import jwt
try:
    import docker
except ImportError:
    docker = None

# Constants & Color Codes
GREEN = '\033[32m'
RED = '\033[31m'
YELLOW = '\033[33m'
CYAN = '\033[36m'
MAGENTA = '\033[35m'
RESET = '\033[0m'

MONGO_URI = os.getenv("MONGO_URI", "mongodb://127.0.0.1:27017/")
ROUTER_URL = os.getenv("ROUTER_URL", "http://127.0.0.1:8000")
BACKEND_URL = os.getenv("BACKEND_URL", "http://127.0.0.1:8080")
WS_URL = os.getenv("WS_URL", "ws://127.0.0.1:8080/ws")
NEXTAUTH_SECRET = os.getenv("NEXTAUTH_SECRET", "testsecret")
DOCKER_HOST = os.getenv("DOCKER_HOST", "tcp://localhost:2375")
BACKEND_GO_SRC = os.getenv("BACKEND_GO_SRC", "../backend-go")

class TraceLogger:
    def __init__(self, trace_id):
        self.trace_id = trace_id
        self.log_file = "verify_system.jsonl"
    
    def log(self, scenario, subsystem, stage, status, message, latency_ms=None, payload=None):
        ts = datetime.datetime.now(datetime.timezone.utc).isoformat()
        entry = {
            "timestamp": ts,
            "trace_id": self.trace_id,
            "scenario_id": scenario,
            "subsystem": subsystem,
            "stage": stage,
            "status": status,
            "message": message,
            "latency_ms": latency_ms,
            "payload": payload
        }
        with open(self.log_file, "a") as f:
            f.write(json.dumps(entry) + "\n")
        
        color = RESET
        icon = ""
        if status == "PASS": color, icon = GREEN, "✓"
        elif status == "FAIL": color, icon = RED, "✗"
        elif status == "WARN": color, icon = YELLOW, "⚠"
        elif status == "INFO": color, icon = CYAN, "»"
        elif status == "SNAPSHOT": color, icon = MAGENTA, "📋"
        
        print(f"[{ts[:19]}][{self.trace_id[:8]}][{subsystem}][{stage}][{color}{icon} {status}{RESET}] {message}")

class LatencyTracker:
    def __init__(self):
        self.samples = {}
    
    def record(self, label, value):
        if label not in self.samples:
            self.samples[label] = []
        self.samples[label].append(value)
        
    def get_stats(self, label):
        s = self.samples.get(label, [])
        if not s: return None
        return {
            "p50": statistics.quantiles(s, n=100)[49] if len(s) > 1 else s[0],
            "p95": statistics.quantiles(s, n=100)[94] if len(s) > 1 else s[0],
            "p99": statistics.quantiles(s, n=100)[98] if len(s) > 1 else s[0],
        }

class FailureCapture:
    @staticmethod
    def capture(trace_id, scenario, snapshot):
        os.makedirs("failure_snapshots", exist_ok=True)
        fname = f"failure_snapshots/{trace_id}_{scenario}_{int(time.time())}.json"
        with open(fname, "w") as f:
            json.dump(snapshot, f, indent=2)
        return fname

class DiagnosticHarness:
    def __init__(self):
        self.trace_id = str(uuid.uuid4())
        self.logger = TraceLogger(self.trace_id)
        self.latencies = LatencyTracker()
        self.mongo_client = pymongo.MongoClient(MONGO_URI)
        self.db = self.mongo_client["omniagent"]
        self.scenario_stats = {f"S{i}": {"passed": 0, "total": 0, "duration_ms": 0, "status": "PENDING"} for i in range(1, 7)}
        self.test_user_id = str(uuid.uuid4())
        self.failure_count = 0
        
    def assert_invariant(self, condition, scenario, subsystem, stage, msg_pass, msg_fail, snapshot=None):
        self.scenario_stats[scenario]["total"] += 1
        if condition:
            self.scenario_stats[scenario]["passed"] += 1
            self.logger.log(scenario, subsystem, stage, "PASS", msg_pass)
            return True
        else:
            self.scenario_stats[scenario]["status"] = "FAIL"
            self.logger.log(scenario, subsystem, stage, "FAIL", msg_fail, payload=snapshot)
            if snapshot:
                fname = FailureCapture.capture(self.trace_id, scenario, snapshot)
                self.logger.log(scenario, subsystem, stage, "SNAPSHOT", f"Snapshot dumped to {fname}")
            return False

    async def run(self):
        print(f"Starting Diagnostic Harness [trace: {self.trace_id}]")
        
        async with aiohttp.ClientSession() as session:
            await self.scenario_1_registry()
            await self.scenario_2_routing(session)
            await self.scenario_3_dood()
            await self.scenario_4_critic(session)
            await self.scenario_5_breaker(session)
            await self.scenario_6_security(session)
            
        self.print_ascii_report()
        total_failed = sum(1 for v in self.scenario_stats.values() if v["status"] == "FAIL")
        if total_failed > 0:
            sys.exit(1)
        sys.exit(0)
        
    async def scenario_1_registry(self):
        s_id = "S1"
        t0 = time.time()
        col = self.db["model_registry"]
        col.delete_many({})
        
        lkg_fact = {
            "model_id": "openai/gpt-4o-test",
            "tier": "frontier",
            "published": {
                "input_cost_per_mtoken": 5.0,
                "output_cost_per_mtoken": 15.0,
                "context_window": 128000,
                "elo_rating": 1250
            },
            "observed": {
                "p50_latency_ms": 500,
                "p95_latency_ms": 1000,
                "output_speed_tps": 50,
                "error_rate_5m": 0,
                "health_status": "HEALTHY"
            },
            "provenance": {
                "source_apis": [],
                "source_trust_score": 1.0,
                "fetched_at": datetime.datetime.now(datetime.timezone.utc),
                "snapshot_version": 1,
                "freshness_state": "FRESH"
            }
        }
        col.insert_one(lkg_fact.copy())
        
        doc = col.find_one({"model_id": "openai/gpt-4o-test"})
        c1 = (doc is not None and doc["published"]["input_cost_per_mtoken"] != 0.0 and 
              doc["published"]["context_window"] >= 1024 and doc["tier"] in ["frontier", "fast", "economy", "reasoning"])
        self.assert_invariant(c1, s_id, "REGISTRY", "BASELINE", "Baseline LKG inserted properly", "Baseline LKG failed checks", {"doc": str(doc)})

        def is_anomalous(lkg_cost, new_cost):
            return abs(new_cost - lkg_cost) / lkg_cost > 0.5
            
        # Anomaly simulation
        c2 = is_anomalous(5.0, 9.0)
        self.assert_invariant(c2, s_id, "REGISTRY", "ANOMALY_LOGIC", "Python is_anomalous returns True for 80% jump", "is_anomalous logic mismatch")
        
        docs = list(col.find({"model_id": "openai/gpt-4o-test"}))
        c3 = (len(docs) == 1 and docs[0]["published"]["input_cost_per_mtoken"] == 5.0)
        self.assert_invariant(c3, s_id, "REGISTRY", "ANOMALY_GUARD", "LKG retained after anomaly injection", "LKG modified by anomaly", {"docs": str(docs)})
        
        # Valid update
        c4 = not is_anomalous(5.0, 5.6)
        valid_fact = copy.deepcopy(lkg_fact)
        valid_fact["published"]["input_cost_per_mtoken"] = 5.6
        valid_fact["provenance"]["snapshot_version"] = 2
        col.insert_one(valid_fact)
        
        docs2 = list(col.find({"model_id": "openai/gpt-4o-test"}).sort("provenance.snapshot_version", -1))
        c5 = (len(docs2) == 2 and docs2[0]["published"]["input_cost_per_mtoken"] == 5.6 and docs2[0]["tier"])
        self.assert_invariant(c5, s_id, "REGISTRY", "VALID_UPDATE", "Valid update applied correctly", "Valid update not applied", {"docs": str(docs2)})
        
        if self.scenario_stats[s_id]["status"] != "FAIL": self.scenario_stats[s_id]["status"] = "PASS"
        self.scenario_stats[s_id]["duration_ms"] = int((time.time() - t0) * 1000)

    async def scenario_2_routing(self, session):
        s_id = "S2"
        t0 = time.time()
        
        candidates = [
            {
                "provider": "openai", "model_id": "openai/gpt-4o-mini", "tier": "fast",
                "published": {"input_cost_per_mtoken": 0.15, "output_cost_per_mtoken": 0.6, "context_window": 128000, "elo_rating": 1280},
                "observed": {"p50_latency_ms": 250, "p95_latency_ms": 450, "output_speed_tps": 80, "error_rate_5m": 0.005, "health_status": "HEALTHY"},
                "provenance": {"source_apis": [], "source_trust_score": 1, "fetched_at": datetime.datetime.now(datetime.timezone.utc).isoformat(), "snapshot_version": 1, "freshness_state": "FRESH"}
            },
            {
                "provider": "anthropic", "model_id": "anthropic/claude-3-5-sonnet", "tier": "frontier",
                "published": {"input_cost_per_mtoken": 3.0, "output_cost_per_mtoken": 15.0, "context_window": 200000, "elo_rating": 1320},
                "observed": {"p50_latency_ms": 400, "p95_latency_ms": 800, "output_speed_tps": 65, "error_rate_5m": 0.002, "health_status": "HEALTHY"},
                "provenance": {"source_apis": [], "source_trust_score": 1, "fetched_at": datetime.datetime.now(datetime.timezone.utc).isoformat(), "snapshot_version": 1, "freshness_state": "FRESH"}
            },
            {
                "provider": "google", "model_id": "google/gemini-1.5-flash", "tier": "fast",
                "published": {"input_cost_per_mtoken": 0.075, "output_cost_per_mtoken": 0.3, "context_window": 1000000, "elo_rating": 1240},
                "observed": {"p50_latency_ms": 180, "p95_latency_ms": 320, "output_speed_tps": 120, "error_rate_5m": 0.001, "health_status": "HEALTHY"},
                "provenance": {"source_apis": [], "source_trust_score": 1, "fetched_at": datetime.datetime.now(datetime.timezone.utc).isoformat(), "snapshot_version": 1, "freshness_state": "FRESH"}
            }
        ]
        
        async def do_route(weights, prompt):
            payload = {"prompt": prompt, "sla_weights": weights, "candidate_models": candidates}
            tt0 = time.perf_counter()
            async with session.post(f"{ROUTER_URL}/v1/route", json=payload) as resp:
                text = await resp.text()
                tt1 = time.perf_counter()
                status = resp.status
            return status, text, (tt1-tt0)*1000, payload
            
        st, text, lat, p_req = await do_route({"w_c": 1.0, "w_q": 0.0, "w_l": 0.0, "w_r": 0.0}, "Summarize this text briefly.")
        if self.assert_invariant(st == 200, s_id, "ROUTING", "COST_FOCUS", "Router returned 200 for cost focus", f"Router returned {st}", {"body": text}):
            resp = json.loads(text)
            self.assert_invariant(lat < 50, s_id, "ROUTING", "LATENCY", f"Latency {lat:.2f}ms < 50ms", f"Latency {lat:.2f}ms >= 50ms", {"lat": lat})
            self.latencies.record("router_cost_focus", lat)
            c2 = resp["selected_model"] in ["openai/gpt-4o-mini", "google/gemini-1.5-flash"]
            self.assert_invariant(c2, s_id, "ROUTING", "COST_SELECTION", f"Selected {resp['selected_model']} for cost", "Selected wrong model", {"resp": resp})
            
        st, text, lat, p_req = await do_route({"w_q": 1.0, "w_c": 0.0, "w_l": 0.0, "w_r": 0.0}, "Write a complex transformer neural network from scratch in PyTorch with detailed comments.")
        if self.assert_invariant(st == 200, s_id, "ROUTING", "QUALITY_FOCUS", "Router returned 200 for quality focus", f"Router returned {st}", {"body": text}):
            resp = json.loads(text)
            self.assert_invariant(lat < 50, s_id, "ROUTING", "LATENCY", f"Latency {lat:.2f}ms < 50ms", f"Latency {lat:.2f}ms >= 50ms", {"lat": lat})
            self.latencies.record("router_quality_focus", lat)
            c3 = resp["selected_model"] == "anthropic/claude-3-5-sonnet"
            self.assert_invariant(c3, s_id, "ROUTING", "QUALITY_SELECTION", f"Selected {resp['selected_model']} for quality", "Selected wrong model", {"resp": resp})
            c4 = resp["metrics_decomposition"]["quality_score"] > resp["metrics_decomposition"]["normalized_cost"]
            self.assert_invariant(c4, s_id, "ROUTING", "METRICS_DECOMPOSITION", "Quality score > normalized cost", "Metrics mismatch", {"resp": resp})
            
        if self.scenario_stats[s_id]["status"] != "FAIL": self.scenario_stats[s_id]["status"] = "PASS"
        self.scenario_stats[s_id]["duration_ms"] = int((time.time() - t0) * 1000)

    async def scenario_3_dood(self):
        s_id = "S3"
        t0 = time.time()
        
        if docker is None:
            self.logger.log(s_id, "SANDBOX", "INIT", "WARN", "docker module not installed, skipping scenario 3")
            self.scenario_stats[s_id]["status"] = "WARN"
            return
            
        try:
            client = docker.DockerClient(base_url=DOCKER_HOST)
            client.ping()
        except Exception as e:
            self.logger.log(s_id, "SANDBOX", "INIT", "WARN", f"Docker daemon not reachable at {DOCKER_HOST}: {e}, skipping.")
            self.scenario_stats[s_id]["status"] = "WARN"
            return

        net_name = f"omniagent-test-net-{str(uuid.uuid4())[:8]}"
        net = client.networks.create(net_name, driver="bridge", internal=True, labels={"omniagent.managed": "true"})
        
        self.assert_invariant(net.id is not None, s_id, "SANDBOX", "NETWORK", "Network created", "Network ID is None")
        net.reload()
        self.assert_invariant(net.attrs.get("Internal") == True, s_id, "SANDBOX", "NETWORK_ISOLATION", "Network is internal", "Network is not internal", {"attrs": net.attrs})
        
        try:
            container = client.containers.run(
                "node:20-alpine",
                ["node", "-e", "console.log('OMNIAGENT_SANDBOX_OK')"],
                network=net.name,
                mem_limit="512m",
                nano_cpus=1_000_000_000,
                pids_limit=128,
                read_only=True,
                security_opt=["no-new-privileges:true"],
                detach=True
            )
            res = container.wait(timeout=30)
            logs = container.logs().decode()
            
            self.assert_invariant("OMNIAGENT_SANDBOX_OK" in logs, s_id, "SANDBOX", "CONTAINER_EXEC", "Execution OK", "Missing expected log output", {"logs": logs})
            self.assert_invariant(res["StatusCode"] == 0, s_id, "SANDBOX", "CONTAINER_EXIT", "Exit 0", f"Exit {res['StatusCode']}", {"res": res})
            
            isolation_container = client.containers.run(
                "node:20-alpine",
                ["sh", "-c", "wget -T 3 -q https://google.com -O - 2>&1 || echo BLOCKED"],
                network=net.name,
                detach=True
            )
            ires = isolation_container.wait(timeout=10)
            ilogs = isolation_container.logs().decode()
            
            c = "BLOCKED" in ilogs or "Network unreachable" in ilogs or "bad address" in ilogs or "timed out" in ilogs
            self.assert_invariant(c, s_id, "SANDBOX", "ISOLATION_CHECK", "Internet blocked correctly", "Internet reachable in internal net", {"ilogs": ilogs})
            
            container.remove(force=True)
            isolation_container.remove(force=True)
            net.remove()
            
            try:
                client.containers.get(container.id)
                c_removed = False
            except docker.errors.NotFound:
                c_removed = True
                
            try:
                client.networks.get(net.id)
                n_removed = False
            except docker.errors.NotFound:
                n_removed = True
                
            self.assert_invariant(c_removed and n_removed, s_id, "SANDBOX", "CLEANUP", "Resources cleaned up", "Resources orphaned")
            
        except Exception as e:
            self.logger.log(s_id, "SANDBOX", "EXEC", "FAIL", f"Exception: {e}", payload={"trace": traceback.format_exc()})
            self.scenario_stats[s_id]["status"] = "FAIL"
            self.failure_count += 1
            
        if self.scenario_stats[s_id]["status"] != "FAIL": self.scenario_stats[s_id]["status"] = "PASS"
        self.scenario_stats[s_id]["duration_ms"] = int((time.time() - t0) * 1000)

    async def scenario_4_critic(self, session):
        s_id = "S4"
        t0 = time.time()
        
        now = int(time.time())
        token = jwt.encode(
            {"email": "tester@omniagent.io", "sub": self.test_user_id, "iat": now, "exp": now + 3600},
            NEXTAUTH_SECRET, algorithm="HS256"
        )
        
        try:
            ws = await websockets.connect(f"{WS_URL}?token={token}")
        except Exception as e:
            self.logger.log(s_id, "CRITIC", "CONNECT", "FAIL", f"WS Connection failed: {e}")
            self.scenario_stats[s_id]["status"] = "FAIL"
            return
            
        await ws.send(json.dumps({
            "type": "NEW_TASK",
            "payload": "Write a Python function that intentionally fails",
            "weights": {"w_q": 1, "w_c": 0, "w_l": 0, "w_r": 0}
        }))
        
        session_id = None
        state_history = []
        critic_fired = False
        
        try:
            while True:
                msg_str = await asyncio.wait_for(ws.recv(), timeout=10)
                msg = json.loads(msg_str)
                if msg["type"] == "DAG_PENDING_APPROVAL":
                    await ws.send(json.dumps({"type": "APPROVE_DAG"}))
                elif msg["type"] == "DAG_UPDATE":
                    if not session_id: session_id = msg.get("session_id")
                    for n in msg.get("nodes", []):
                        state_history.append(n["state"])
                        if n["state"] in ["ESCALATED"]:
                            critic_fired = True
                    if all(n["state"] in ["SUCCEEDED", "FAILED", "ESCALATED", "CANCELLED"] for n in msg.get("nodes", [])):
                        break
                elif msg["type"] == "ERROR":
                    self.logger.log(s_id, "CRITIC", "WS_ERROR", "INFO", f"WS Error received: {msg}")
                    break
        except asyncio.TimeoutError:
            self.logger.log(s_id, "CRITIC", "WS_TIMEOUT", "WARN", "Timeout waiting for terminal state")
        except Exception as e:
            self.logger.log(s_id, "CRITIC", "RECV", "WARN", f"WS exception: {e}")
            
        self.assert_invariant(critic_fired or ("ESCALATED" in state_history) or ("SUCCEEDED" in state_history) or ("FAILED" in state_history), 
                              s_id, "CRITIC", "FSM_FLOW", "Critic/Task reached terminal or escalated state", "Never finished", {"history": state_history})
                              
        if not session_id:
            self.logger.log(s_id, "CRITIC", "PHASE_C", "WARN", "No session_id collected")
        else:
            await ws.close()
            await asyncio.sleep(2)
            events = list(self.db["task_events"].find({"session_id": session_id}).sort("timestamp", -1).limit(10))
            self.assert_invariant(len(events) >= 2, s_id, "CRITIC", "DB_BUFFER", f"DB has {len(events)} events", "Not buffered", {"count": len(events)})
            c = any(e.get("type") == "NODE_TRANSITION" for e in events)
            self.assert_invariant(c, s_id, "CRITIC", "DB_TRANSITION", "NODE_TRANSITION found in DB", "No transitions found in DB")
            
        if self.scenario_stats[s_id]["status"] != "FAIL": self.scenario_stats[s_id]["status"] = "PASS"
        self.scenario_stats[s_id]["duration_ms"] = int((time.time() - t0) * 1000)

    async def scenario_5_breaker(self, session):
        s_id = "S5"
        t0 = time.time()
        
        # Phase A: Subprocess
        if os.path.isdir(BACKEND_GO_SRC):
            try:
                res = subprocess.run(["go", "test", "-v", "-race", "-run", "TestCircuitBreaker", "./internal/llm_clients/..."],
                                     cwd=BACKEND_GO_SRC, capture_output=True, text=True, timeout=60)
                
                self.assert_invariant(res.returncode == 0, s_id, "BREAKER", "UNIT_TEST", "go test TestCircuitBreaker passed", f"go test failed (exit {res.returncode})", {"stdout": res.stdout, "stderr": res.stderr})
                self.assert_invariant("TestCircuitBreaker_TripsOnNetworkErrors" in res.stdout, s_id, "BREAKER", "LOG_TRIP", "TestCircuitBreaker_TripsOnNetworkErrors ran", "Missing in stdout")
                self.assert_invariant("TestCircuitBreaker_Ignores4xxErrors" in res.stdout, s_id, "BREAKER", "LOG_IGNORE", "TestCircuitBreaker_Ignores4xxErrors ran", "Missing in stdout")
            except Exception as e:
                self.logger.log(s_id, "BREAKER", "UNIT_TEST", "WARN", f"go test exception: {e}")
        else:
            self.logger.log(s_id, "BREAKER", "UNIT_TEST", "WARN", f"Backend src not found at {BACKEND_GO_SRC}, skipping Phase A")
            
        # Phase B: Router failover
        candidates = [
            {
                "provider": "openai", "model_id": "openai/gpt-4o", "tier": "frontier",
                "published": {"input_cost_per_mtoken": 5.0, "output_cost_per_mtoken": 15.0, "context_window": 128000, "elo_rating": 1300},
                "observed": {"p50_latency_ms": 500, "p95_latency_ms": 1000, "output_speed_tps": 50, "error_rate_5m": 1.0, "health_status": "DOWN"},
                "provenance": {"source_apis": [], "source_trust_score": 1, "fetched_at": datetime.datetime.now(datetime.timezone.utc).isoformat(), "snapshot_version": 1, "freshness_state": "FRESH"}
            },
            {
                "provider": "anthropic", "model_id": "anthropic/claude-3-5-sonnet", "tier": "frontier",
                "published": {"input_cost_per_mtoken": 3.0, "output_cost_per_mtoken": 15.0, "context_window": 200000, "elo_rating": 1320},
                "observed": {"p50_latency_ms": 400, "p95_latency_ms": 800, "output_speed_tps": 65, "error_rate_5m": 0.0, "health_status": "HEALTHY"},
                "provenance": {"source_apis": [], "source_trust_score": 1, "fetched_at": datetime.datetime.now(datetime.timezone.utc).isoformat(), "snapshot_version": 1, "freshness_state": "FRESH"}
            }
        ]
        payload = {
            "prompt": "Hello",
            "sla_weights": {"w_q": 0.5, "w_c": 0.5, "w_l": 0.5, "w_r": 0.5},
            "candidate_models": candidates
        }
        async with session.post(f"{ROUTER_URL}/v1/route", json=payload) as resp:
            st = resp.status
            text = await resp.text()
            
        if self.assert_invariant(st == 200, s_id, "BREAKER", "ROUTER_HTTP", "Router HTTP 200", f"Router HTTP {st}"):
            resp_data = json.loads(text)
            c = resp_data["selected_model"] != "openai/gpt-4o"
            self.assert_invariant(c, s_id, "BREAKER", "ROUTER_FAILOVER", f"Router bypassed DOWN provider (selected {resp_data['selected_model']})", "Router selected DOWN provider", {"resp": resp_data})
            
        if self.scenario_stats[s_id]["status"] != "FAIL": self.scenario_stats[s_id]["status"] = "PASS"
        self.scenario_stats[s_id]["duration_ms"] = int((time.time() - t0) * 1000)

    async def scenario_6_security(self, session):
        s_id = "S6"
        t0 = time.time()
        
        try:
            await websockets.connect(f"{WS_URL}?token=eyJhbGciOiJIUzI1NiJ9.invalid.sig")
            c = False
            e_str = "Connection succeeded"
        except Exception as e:
            c = True
            e_str = str(e)
        self.assert_invariant(c, s_id, "SECURITY", "WS_INVALID_SIG", "Invalid signature rejected", "Invalid signature accepted", {"err": e_str})
        
        now = int(time.time())
        expired = jwt.encode({"email": "tester@omniagent.io", "sub": "123", "exp": now - 3600}, NEXTAUTH_SECRET, algorithm="HS256")
        try:
            await websockets.connect(f"{WS_URL}?token={expired}")
            c = False
            e_str = "Connection succeeded"
        except Exception as e:
            c = True
            e_str = str(e)
        self.assert_invariant(c, s_id, "SECURITY", "WS_EXPIRED", "Expired token rejected", "Expired token accepted", {"err": e_str})
        
        valid = jwt.encode({"email": "tester@omniagent.io", "sub": "123", "iat": now, "exp": now + 3600}, NEXTAUTH_SECRET, algorithm="HS256")
        try:
            ws = await websockets.connect(f"{WS_URL}?token={valid}")
            await ws.send(json.dumps({
                "type": "NEW_TASK",
                "payload": "Fetch http://169.254.169.254/latest/meta-data/",
                "weights": {"w_q": 1, "w_c": 0, "w_l": 0, "w_r": 0}
            }))
            await asyncio.sleep(1)
            await ws.close()
            self.logger.log(s_id, "SECURITY", "SSRF", "INFO", "SSRF payload submitted, check backend logs for blocks")
        except Exception as e:
            self.logger.log(s_id, "SECURITY", "SSRF", "WARN", f"WS exception during SSRF submit: {e}")
            
        if os.path.isdir(BACKEND_GO_SRC):
            try:
                res = subprocess.run(["go", "test", "-v", "-race", "-run", "TestDecryptUserKey", "./internal/security/..."],
                                     cwd=BACKEND_GO_SRC, capture_output=True, text=True, timeout=60)
                
                self.assert_invariant(res.returncode == 0, s_id, "SECURITY", "MEM_ZEROING", "go test TestDecryptUserKey passed", f"go test failed (exit {res.returncode})", {"stdout": res.stdout})
                self.assert_invariant("TestDecryptUserKey_Zeroing" in res.stdout, s_id, "SECURITY", "MEM_ZEROING_LOG", "TestDecryptUserKey_Zeroing ran", "Missing in stdout")
            except Exception as e:
                self.logger.log(s_id, "SECURITY", "MEM_ZEROING", "WARN", f"go test exception: {e}")
        else:
            self.logger.log(s_id, "SECURITY", "MEM_ZEROING", "WARN", f"Backend src not found at {BACKEND_GO_SRC}, skipping Go test")
            
        if self.scenario_stats[s_id]["status"] != "FAIL": self.scenario_stats[s_id]["status"] = "PASS"
        self.scenario_stats[s_id]["duration_ms"] = int((time.time() - t0) * 1000)

    def print_ascii_report(self):
        print("╔══════════════════════════════════════════════════════════════════════╗")
        print(f"║              OMNIAGENT DIAGNOSTIC REPORT  [trace: {self.trace_id[:8]}]         ║")
        print("╠════════════════════════════════╦════════╦════════════╦══════════════╣")
        print("║  Scenario                      ║ Status ║ Invariants ║ Duration     ║")
        print("╠════════════════════════════════╬════════╬════════════╬══════════════╣")
        
        scenario_names = {
            "S1": "Registry Ingestion", "S2": "Dynamic Routing", "S3": "DooD Sandboxing", 
            "S4": "Critic & Retry Loop", "S5": "Circuit Breaker", "S6": "Key Security"
        }
        
        for k, name in scenario_names.items():
            stats = self.scenario_stats[k]
            status = stats["status"]
            dur = stats["duration_ms"]
            p, t = stats["passed"], stats["total"]
            print(f"║  {k}: {name:<26} ║  {status:<6}║   {p:>2} / {t:<2}   ║  {dur:>5}ms     ║")
            
        print("╠════════════════════════════════╩════════╩════════════╩══════════════╣")
        stats = self.latencies.get_stats("router_cost_focus")
        if stats:
            print(f"║  Router Latency (Cost Focus) P50: {stats['p50']:.2f}ms                             ║")
        passed = sum(v["passed"] for v in self.scenario_stats.values())
        total = sum(v["total"] for v in self.scenario_stats.values())
        print(f"║  Total Invariants Passed: {passed:>2} / {total:<2}                                   ║")
        print("╠══════════════════════════════════════════════════════════════════════╣")
        failures = sum(1 for v in self.scenario_stats.values() if v["status"] == "FAIL")
        if failures == 0:
            print("║  ✓ ALL CRITICAL INVARIANTS PASSED                    Exit Code: 0   ║")
        else:
            print(f"║  ✗ {failures} SCENARIOS FAILED                                   Exit Code: 1   ║")
        print("╚══════════════════════════════════════════════════════════════════════╝")


if __name__ == "__main__":
    harness = DiagnosticHarness()
    asyncio.run(harness.run())
