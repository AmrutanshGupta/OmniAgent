import os
import random
import torch
from app.model import SharedEncoderNet

# Safely resolve absolute paths regardless of execution directory
BASE_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
WEIGHTS_PATH = os.path.join(BASE_DIR, "weights", "router_model.pt")

_model = None

def get_router_model():
    """Singleton pattern for thread-safe model loading."""
    global _model
    if _model is None:
        _model = SharedEncoderNet(input_dim=7, num_agents=3)
        if os.path.exists(WEIGHTS_PATH):
            try:
                _model.load_state_dict(torch.load(WEIGHTS_PATH, map_location=torch.device('cpu')))
            except Exception as e:
                print(f"Warning: Could not load weights, using random initialization. Error: {e}")
        _model.eval()
    return _model

def reload_model_weights():
    """Hot-swaps weights in memory after online fine-tuning."""
    global _model
    _model = None
    get_router_model()

def infer_optimal_route(
    prompt_tokens: float, complexity: float, domain_idx: float,
    q_0: float, q_1: float, q_2: float, gpu_util: float,
    w_q: float, w_l: float, w_c: float, epsilon: float = 0.05
):
    model = get_router_model()
    
    # Contextual Bandit Exploration: 5% of the time, force a random route to gather fresh telemetry
    if random.random() < epsilon:
        chosen_agent = random.choice([0, 1, 2])
        return chosen_agent, 0.0, 0.0
        
    input_vector = torch.tensor([[
        prompt_tokens, complexity, domain_idx, q_0, q_1, q_2, gpu_util
    ]], dtype=torch.float32)
    
    with torch.no_grad():
        q_preds, l_preds, c_preds = model(input_vector)
        
    q = q_preds[0]
    l = l_preds[0]
    c = c_preds[0]
    
    # The Optimization Equation: Utility(a) = w_q*Q - w_l*L - w_c*C
    utilities = (w_q * q) - (w_l * l) - (w_c * c)
    chosen_agent = int(torch.argmax(utilities).item())
    
    return chosen_agent, float(l[chosen_agent].item()), float(c[chosen_agent].item())