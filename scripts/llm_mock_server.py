import time
from collections import defaultdict
from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse

app = FastAPI()

# Request counters by provider/path
counters = defaultdict(int)

@app.post("/{full_path:path}")
async def catch_all_post(full_path: str, request: Request):
    counters[full_path] += 1
    
    # 3 consecutive network/5xx failures -> trips breaker.
    # We will fail the first 3 times, then succeed.
    if counters[full_path] <= 3:
        return JSONResponse(status_code=500, content={"error": f"Internal Server Error for {full_path}"})
    
    # Return dummy success
    if "messages" in full_path:  # Anthropic
        return JSONResponse(content={
            "content": [{"text": "Mock Anthropic response"}],
            "usage": {"output_tokens": 10}
        })
    elif "chat/completions" in full_path: # OpenAI / Groq
        return JSONResponse(content={
            "choices": [{"message": {"content": "Mock OpenAI response"}}],
            "usage": {"completion_tokens": 10}
        })
    elif "generateContent" in full_path: # Google
        return JSONResponse(content={
            "candidates": [{"content": {"parts": [{"text": "Mock Google response"}]}}]
        })
    else: # Huggingface
        return JSONResponse(content=[{"generated_text": "Mock HF response"}])

@app.get("/{full_path:path}")
async def catch_all_get(full_path: str, request: Request):
    return JSONResponse(content={"status": "ok", "path": full_path})
