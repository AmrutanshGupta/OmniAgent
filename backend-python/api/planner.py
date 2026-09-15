from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, Field
from typing import List, Dict, Set, cast
from langchain_core.prompts import ChatPromptTemplate
from langchain_openai import ChatOpenAI
from langchain_anthropic import ChatAnthropic
from langchain_google_genai import ChatGoogleGenerativeAI
from langchain_groq import ChatGroq
import logging

router = APIRouter()
logger = logging.getLogger(__name__)

class SubTask(BaseModel):
    id: str = Field(description="Unique identifier for the node, e.g., 'node_1'")
    task: str = Field(description="Detailed description of the task to be performed")
    domain: int = Field(description="Domain Mapping: 0=General, 1=Code, 2=Math, 3=RAG.")
    depends_on: List[str] = Field(description="List of node IDs that must be completed before this node can run")

class DAGPlan(BaseModel):
    plan_name: str
    rationale: str = Field(description="Why this decomposition is optimal")
    completeness_score: float = Field(description="0.0 to 1.0 score of how completely this addresses the user prompt")
    feasibility_score: float = Field(description="0.0 to 1.0 score of how easily parallelizable and feasible this plan is")
    nodes: List[SubTask]

class PlannerRequest(BaseModel):
    prompt: str
    keys: Dict[str, str] = Field(default_factory=dict)
    
class MultiPlanResponse(BaseModel):
    candidates: List[DAGPlan]

def is_acyclic(nodes: List[SubTask]) -> bool:
    """Validate topological sort / detect cycles in the DAG."""
    adj: Dict[str, List[str]] = {node.id: [] for node in nodes}
    for node in nodes:
        for dep in node.depends_on:
            if dep not in adj:
                return False 
            adj[dep].append(node.id)

    visited = set()
    rec_stack = set()

    def dfs(n: str) -> bool:
        visited.add(n)
        rec_stack.add(n)
        for neighbor in adj.get(n, []):
            if neighbor not in visited:
                if dfs(neighbor):
                    return True
            elif neighbor in rec_stack:
                return True
        rec_stack.remove(n)
        return False

    for node_id in adj:
        if node_id not in visited:
            if dfs(node_id):
                return False # Cycle detected

    return True

def score_plan(plan: DAGPlan) -> float:

    n = len(plan.nodes)
    size_penalty = 1.0
    if n < 2 or n > 8:
        size_penalty = 0.5
    
    return ((plan.completeness_score + plan.feasibility_score) / 2.0) * size_penalty

@router.post("/plan", response_model=List[SubTask])
async def generate_plan(request: PlannerRequest):
    # Enforce budget/keys
    if not request.keys:
        raise HTTPException(status_code=400, detail="No valid API keys provided for planning. Task dispatch aborted due to budget/credentials constraint.")

    llm = None
    if "anthropic" in request.keys and request.keys["anthropic"]:
        llm = ChatAnthropic(model="claude-3-5-sonnet-20240620", api_key=request.keys["anthropic"])
    elif "openai" in request.keys and request.keys["openai"]:
        llm = ChatOpenAI(model="gpt-4o", api_key=request.keys["openai"])
    elif "groq" in request.keys and request.keys["groq"]:
        llm = ChatGroq(model="llama3-70b-8192", api_key=request.keys["groq"])
    elif "google" in request.keys and request.keys["google"]:
        llm = ChatGoogleGenerativeAI(model="gemini-1.5-flash", api_key=request.keys["google"])
    else:
        raise HTTPException(status_code=400, detail="Task dispatch aborted: Insufficient budget or no keys available.")

    structured_llm = llm.with_structured_output(MultiPlanResponse)

    system_prompt = """You are the OmniAgent Master Architect. Break down the user's complex request into a Directed Acyclic Graph (DAG) of sub-tasks.
To perform a Tree-of-Thoughts exploration, generate exactly 3 alternative DAG plans (e.g. one sequential, one highly parallel, one hybrid).
Score them for completeness and feasibility (0.0 to 1.0).
Domain Mapping: 0=General, 1=Code (writing code/scripts), 2=Math, 3=RAG.
Ensure each node's `depends_on` only references valid previously defined node `id`s and contains NO cyclic dependencies.
"""

    prompt_template = ChatPromptTemplate.from_messages([
        ("system", system_prompt),
        ("human", "{user_request}")
    ])

    chain = prompt_template | structured_llm

    try:
        result: MultiPlanResponse = cast(
            MultiPlanResponse,
            await chain.ainvoke({"user_request": request.prompt})
        )
        
        # 1. Topological validation and utility scoring
        valid_plans = []
        for plan in result.candidates:
            if not is_acyclic(plan.nodes):
                logger.warning(f"Pruned plan {plan.plan_name} due to cyclic dependency or invalid edges.")
                continue
            
            utility = score_plan(plan)
            # Prune below utility threshold
            if utility < 0.6:
                logger.warning(f"Pruned plan {plan.plan_name} due to low utility score: {utility}")
                continue
                
            valid_plans.append((utility, plan))
            
        if not valid_plans:
            raise ValueError("All generated branches were pruned due to invalid topology or low utility.")
            
        # 2. Select the best plan
        valid_plans.sort(key=lambda x: x[0], reverse=True)
        best_plan = valid_plans[0][1]
        
        logger.info(f"Selected best plan '{best_plan.plan_name}' with {len(best_plan.nodes)} nodes.")
        
        return best_plan.nodes
    except Exception as e:
        logger.error(f"Planning failed: {str(e)}")
        raise HTTPException(status_code=500, detail=str(e))
