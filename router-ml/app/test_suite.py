import pytest
import torch
from fastapi.testclient import TestClient

from app.main import app
from app.predict import infer_optimal_route
from app.model import SharedEncoderNet

# ---------------------------------------------------------
# Fixtures
# ---------------------------------------------------------
@pytest.fixture
def client():
    """Provides a reusable FastAPI TestClient for endpoint testing."""
    return TestClient(app)

# ---------------------------------------------------------
# Test Cases
# ---------------------------------------------------------

def test_pytorch_model_architecture():
    """[1/4] Verifies the PyTorch model instantiates and outputs correct tensor shapes."""
    model = SharedEncoderNet(input_dim=7, num_agents=3)
    dummy_input = torch.rand(1, 7)
    q, l, c = model(dummy_input)
    
    # Asserting shapes dynamically 
    assert q.shape == (1, 3), f"Expected Quality tensor shape (1, 3), got {q.shape}"
    assert l.shape == (1, 3), f"Expected Latency tensor shape (1, 3), got {l.shape}"
    assert c.shape == (1, 3), f"Expected Cost tensor shape (1, 3), got {c.shape}"

def test_dynamic_utility_equation():
    """[2/4] Verifies the mathematical routing logic and constraint boundaries."""
    agent_id, lat, cost = infer_optimal_route(
        prompt_tokens=1500, complexity=5.0, domain_idx=1,
        q_0=10, q_1=2, q_2=0, gpu_util=0.8,
        w_q=0.5, w_l=0.3, w_c=0.2, 
        epsilon=0.0 # Force deterministic output (disable random exploration)
    )
    
    assert agent_id in [0, 1, 2], f"Invalid agent ID selected: {agent_id}"
    assert lat >= 0, f"Latency prediction must be non-negative, got {lat}"
    assert cost >= 0, f"Cost prediction must be non-negative, got {cost}"

def test_routing_endpoint(client):
    """[3/4] Tests the main FastAPI inference endpoint for structural contract compliance."""
    payload = {
        "prompt_tokens": 1200, "complexity_heuristic": 8.5, "task_domain_idx": 0,
        "q_0": 5, "q_1": 5, "q_2": 2, "gpu_util": 0.90,
        "w_q": 0.8, "w_l": 0.1, "w_c": 0.1
    }
    response = client.post("/v1/route", json=payload)
    
    assert response.status_code == 200, f"API failed with status {response.status_code}"
    
    data = response.json()
    assert "chosen_agent_id" in data, "Missing 'chosen_agent_id' in response"
    assert "expected_latency" in data, "Missing 'expected_latency' in response"
    assert "expected_cost" in data, "Missing 'expected_cost' in response"
    assert data["chosen_agent_id"] in [0, 1, 2]

def test_telemetry_pipeline(client):
    """[4/4] Tests the asynchronous background online tuning enqueue endpoint."""
    telemetry_payload = [{
        "tokens": 500, "comp": 2.0, "domain": 1,
        "q0": 0, "q1": 1, "q2": 1, "gpu": 0.5,
        "true_q": [1.0, 0.8, 0.5], "true_l": [1.1, 0.6, 0.1], "true_c": [0.01, 0.005, 0.001]
    }]
    response = client.post("/v1/telemetry/sync", json=telemetry_payload)
    
    assert response.status_code == 200, f"Expected 200 OK, got {response.status_code}"
    
    data = response.json()
    assert data.get("status") == "enqueued", f"Expected status 'enqueued', got {data.get('status')}"
    assert data.get("batch_size") == 1, "Batch size mismatch"