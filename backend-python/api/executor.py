from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, Field
from typing import Dict, Any, Optional
from langchain_core.prompts import ChatPromptTemplate
from langchain_openai import ChatOpenAI
from langchain_anthropic import ChatAnthropic
from langchain_google_genai import ChatGoogleGenerativeAI
from langchain_groq import ChatGroq
from langchain.agents import create_tool_calling_agent, AgentExecutor
from tools.context_search import get_context_search_tool

router = APIRouter()

class ExecutorRequest(BaseModel):
    session_id: str
    node_id: str
    task: str
    original_prompt: str
    tier: str
    keys: Dict[str, str] = Field(default_factory=dict)

class ExecutorResponse(BaseModel):
    result: str
    tokens_used: int

@router.post("/execute-node", response_model=ExecutorResponse)
async def execute_node(request: ExecutorRequest):
    llm = None
    # Use the specific tier requested by the router
    if request.tier in ["gpt-4o", "openai"] and "openai" in request.keys and request.keys["openai"]:
        llm = ChatOpenAI(model="gpt-4o", api_key=request.keys["openai"])
    elif request.tier in ["sonnet", "anthropic"] and "anthropic" in request.keys and request.keys["anthropic"]:
        llm = ChatAnthropic(model="claude-3-5-sonnet-20240620", api_key=request.keys["anthropic"])
    elif request.tier in ["groq"] and "groq" in request.keys and request.keys["groq"]:
        llm = ChatGroq(model="llama3-70b-8192", api_key=request.keys["groq"])
    elif request.tier in ["flash", "google", "hf"] and "google" in request.keys and request.keys["google"]:
        # fallback hf to google for now if hf is missing
        llm = ChatGoogleGenerativeAI(model="gemini-1.5-flash", api_key=request.keys["google"])
    else:
        # Fallback cascade logic here if the specific tier is missing but we have others
        if "anthropic" in request.keys and request.keys["anthropic"]:
            llm = ChatAnthropic(model="claude-3-5-sonnet-20240620", api_key=request.keys["anthropic"])
        elif "openai" in request.keys and request.keys["openai"]:
            llm = ChatOpenAI(model="gpt-4o", api_key=request.keys["openai"])
        elif "google" in request.keys and request.keys["google"]:
            llm = ChatGoogleGenerativeAI(model="gemini-1.5-flash", api_key=request.keys["google"])
        elif "groq" in request.keys and request.keys["groq"]:
            llm = ChatGroq(model="llama3-70b-8192", api_key=request.keys["groq"])
        else:
            raise HTTPException(status_code=400, detail="No valid API keys available for execution")

    # Set up memory and tools
    context_tool = get_context_search_tool(request.session_id)
    tools = [context_tool]

    system_prompt = """You are a specialized worker agent in an OmniAgent orchestration network.
Complete the following specific sub-task concisely and accurately. 
If you need context from previously executed nodes in this session, use the context_search tool."""

    prompt = ChatPromptTemplate.from_messages([
        ("system", system_prompt),
        ("human", "Global Context (The User's Goal): {original_prompt}\n\nYour Specific Sub-Task: {task}"),
        ("placeholder", "{agent_scratchpad}")
    ])

    agent = create_tool_calling_agent(llm, tools, prompt)
    agent_executor = AgentExecutor(agent=agent, tools=tools, verbose=True)

    try:
        # We need to wrap with langfuse later, but Langfuse can be attached via callbacks or env vars
        result = await agent_executor.ainvoke({
            "original_prompt": request.original_prompt,
            "task": request.task
        })
        
        output = result.get("output", "")
        # Dummy token count for now; a real implementation would extract from LLM response metadata or langfuse
        tokens_used = len(output) // 4 

        # Store result in MongoDB
        from tools.context_search import store_node_result
        await store_node_result(request.session_id, request.node_id, output)

        return ExecutorResponse(result=output, tokens_used=tokens_used)
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))
