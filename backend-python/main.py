from fastapi import FastAPI
from contextlib import asynccontextmanager
from api.planner import router as planner_router
from api.executor import router as executor_router
from router.elo_router import router as elo_router, start_elo_cron
import logging
import json
from datetime import datetime, timezone

class JSONFormatter(logging.Formatter):
    def format(self, record):
        log_record = {
            "time": datetime.now(timezone.utc).isoformat(),
            "level": record.levelname,
            "message": record.getMessage(),
            "name": record.name
        }
        if record.exc_info:
            log_record["exc_info"] = self.formatException(record.exc_info)
        return json.dumps(log_record)

handler = logging.StreamHandler()
handler.setFormatter(JSONFormatter())
logging.basicConfig(level=logging.INFO, handlers=[handler], force=True)

@asynccontextmanager
async def lifespan(app: FastAPI):
    # Start the background task to fetch ELO scores
    scheduler = start_elo_cron()
    yield
    # Shutdown logic
    if scheduler:
        scheduler.shutdown()

app = FastAPI(title="OmniAgent Python Backend", lifespan=lifespan)

app.include_router(planner_router, prefix="/v1")
app.include_router(executor_router, prefix="/v1")
app.include_router(elo_router, prefix="/v1/router")

@app.get("/health")
async def health_check():
    return {"status": "ok"}
