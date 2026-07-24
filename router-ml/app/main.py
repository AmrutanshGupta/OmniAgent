import os
import torch
import torch.nn as nn
from fastapi import FastAPI, BackgroundTasks, HTTPException
from pydantic import BaseModel, Field
from app.predict import infer_optimal_route, reload_model_weights, WEIGHTS_PATH
from app.model import SharedEncoderNet

app = FastAPI(title="OmniAgent ML Router", version="1.0.0")

class RoutingRequest(BaseModel):
    prompt_tokens: int = Field(..., description="Task token size")
    complexity_heuristic: float = Field(..., description="Cognitive difficulty rating")
    task_domain_idx: int = Field(..., description="0=General, 1=Code, 2=Math, 3=RAG")
    q_0: int = Field(..., description="Heavy Tier queue depth")
    q_1: int = Field(..., description="Specialist Tier queue depth")
    q_2: int = Field(..., description="Flash Tier queue depth")
    gpu_util: float = Field(..., description="Cluster saturation (0.0 to 1.0)")
    w_q: float = Field(default=0.6, description="SLA Quality Weight")
    w_l: float = Field(default=0.2, description="SLA Latency Penalty")
    w_c: float = Field(default=0.2, description="SLA Cost Penalty")

class RoutingResponse(BaseModel):
    chosen_agent_id: int
    expected_latency: float
    expected_cost: float

def execute_background_online_tuning(telemetry_batch: list):
    """Background task to slightly adjust model weights based on live data."""
    try:
        model = SharedEncoderNet(input_dim=7, num_agents=3)
        if os.path.exists(WEIGHTS_PATH):
            model.load_state_dict(torch.load(WEIGHTS_PATH, map_location='cpu'))
            
        optimizer = torch.optim.AdamW(model.parameters(), lr=1e-3)
        mse = nn.MSELoss()
        bce = nn.BCELoss()
        
        inputs, t_q, t_l, t_c = [], [], [], []
        for s in telemetry_batch:
            inputs.append([s['tokens'], s['comp'], s['domain'], s['q0'], s['q1'], s['q2'], s['gpu']])
            t_q.append(s['true_q'])
            t_l.append(s['true_l'])
            t_c.append(s['true_c'])
            
        inputs = torch.tensor(inputs, dtype=torch.float32)
        model.train()
        optimizer.zero_grad()
        
        q_pred, l_pred, c_pred = model(inputs)
        loss = bce(q_pred, torch.tensor(t_q, dtype=torch.float32)) + \
               0.5 * mse(l_pred, torch.tensor(t_l, dtype=torch.float32)) + \
               0.5 * mse(c_pred, torch.tensor(t_c, dtype=torch.float32))
               
        loss.backward()
        optimizer.step()
        
        os.makedirs(os.path.dirname(WEIGHTS_PATH), exist_ok=True)
        torch.save(model.state_dict(), WEIGHTS_PATH)
        reload_model_weights()
    except Exception as e:
        print(f"Background tuning failed: {e}")

@app.post("/v1/route", response_model=RoutingResponse)
async def route_task(req: RoutingRequest):
    try:
        agent_id, lat, cost = infer_optimal_route(
            req.prompt_tokens, req.complexity_heuristic, req.task_domain_idx,
            req.q_0, req.q_1, req.q_2, req.gpu_util, req.w_q, req.w_l, req.w_c
        )
        return RoutingResponse(chosen_agent_id=agent_id, expected_latency=lat, expected_cost=cost)
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/v1/telemetry/sync")
async def process_batch_sync(batch: list[dict], bg_tasks: BackgroundTasks):
    if not batch: return {"status": "ignored"}
    bg_tasks.add_task(execute_background_online_tuning, batch)
    return {"status": "enqueued", "batch_size": len(batch)}