from fastapi import APIRouter
from pydantic import BaseModel, Field
from apscheduler.schedulers.background import BackgroundScheduler
import logging
from typing import List, Dict, Any

router = APIRouter()
logger = logging.getLogger(__name__)

def fetch_elo_scores():
    """Background task to fetch and update ELO scores."""
    logger.info("Fetching ELO scores from leaderboard...")
    # Kept for compatibility, actual logic moved to Go backend's ingestion worker.
    pass

def start_elo_cron():
    scheduler = BackgroundScheduler()
    scheduler.add_job(fetch_elo_scores, 'cron', hour=0, minute=0)
    scheduler.start()
    return scheduler

class SLAWeights(BaseModel):
    w_q: float
    w_c: float
    w_l: float
    w_r: float

class DynamicRoutingRequest(BaseModel):
    prompt: str
    sla_weights: SLAWeights
    candidate_models: List[Dict[str, Any]]

class MetricsDecomposition(BaseModel):
    quality_score: float
    normalized_cost: float
    normalized_latency: float
    reliability_score: float

class DynamicRoutingResponse(BaseModel):
    selected_model: str
    expected_utility: float
    metrics_decomposition: MetricsDecomposition
    ranked_alternatives: List[str]

@router.post("/route", response_model=DynamicRoutingResponse)
async def route_task(request: DynamicRoutingRequest):
    candidates = request.candidate_models
    
    if not candidates:
        logger.warning("No candidate models provided. Falling back to default.")
        return DynamicRoutingResponse(
            selected_model="flash",
            expected_utility=0.0,
            metrics_decomposition=MetricsDecomposition(
                quality_score=0.0,
                normalized_cost=0.0,
                normalized_latency=0.0,
                reliability_score=0.0
            ),
            ranked_alternatives=[]
        )

    # 1. Compute max values for normalization
    max_elo = 1.0
    max_cost = 0.0001
    max_latency = 1.0

    for c in candidates:
        pub = c.get("published", {})
        obs = c.get("observed", {})
        
        elo = pub.get("elo_rating", 0)
        cost = pub.get("input_cost_per_mtoken", 0.0)
        latency = obs.get("p50_latency_ms", 0)
        
        if elo > max_elo: max_elo = elo
        if cost > max_cost: max_cost = cost
        if latency > max_latency: max_latency = latency

    # 2. Score models
    scored_models = []
    
    w = request.sla_weights
    
    for c in candidates:
        pub = c.get("published", {})
        obs = c.get("observed", {})
        
        elo = pub.get("elo_rating", 0)
        cost = pub.get("input_cost_per_mtoken", 0.0)
        latency = obs.get("p50_latency_ms", 0)
        error_rate = obs.get("error_rate_5m", 0.0)
        
        norm_quality = elo / max_elo if max_elo > 0 else 0.0
        norm_cost = cost / max_cost if max_cost > 0 else 0.0
        norm_latency = latency / max_latency if max_latency > 0 else 0.0
        reliability = max(0.0, 1.0 - error_rate)
        
        utility = (w.w_q * norm_quality) - (w.w_c * norm_cost) - (w.w_l * norm_latency) + (w.w_r * reliability)
        
        tier = c.get("tier", "economy") # Default to economy if missing
        
        metrics = MetricsDecomposition(
            quality_score=norm_quality,
            normalized_cost=norm_cost,
            normalized_latency=norm_latency,
            reliability_score=reliability
        )
        
        scored_models.append({
            "model_id": c.get("model_id"),
            "tier": tier,
            "utility": utility,
            "metrics": metrics
        })

    # 3. Sort by utility descending
    scored_models.sort(key=lambda x: x["utility"], reverse=True)
    
    best = scored_models[0]
    
    # Map tier string if needed, or return the tier as the agent selection. 
    # The Go backend uses MLTierInt in some places, but also maps strings. 
    # We will return the tier name directly which `GetOptimalRoute` will map.
    selected_tier = best["tier"]
    
    alternatives = [m["tier"] for m in scored_models[1:]]
    
    # De-duplicate alternatives while preserving order
    seen = set([selected_tier])
    unique_alts = []
    for a in alternatives:
        if a not in seen:
            unique_alts.append(a)
            seen.add(a)
    
    return DynamicRoutingResponse(
        selected_model=selected_tier,
        expected_utility=best["utility"],
        metrics_decomposition=best["metrics"],
        ranked_alternatives=unique_alts
    )
