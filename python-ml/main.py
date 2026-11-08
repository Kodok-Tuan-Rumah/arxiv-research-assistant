import logging
import os
import uuid
from contextlib import asynccontextmanager

import state
from engine.retrieval.qdrant_client import ensure_collection, get_client
from fastapi import FastAPI, Request, status
from fastapi.responses import JSONResponse
from prometheus_fastapi_instrumentator import Instrumentator
from routers import chunk, embed, ingest, papers, query
from sentence_transformers import SentenceTransformer

# Structured logging setup
logging.basicConfig(
    level=logging.INFO,
    format='{"timestamp": "%(asctime)s", "level": "%(levelname)s", "service": "arxiv-ml", "message": "%(message)s"}',
    datefmt="%Y-%m-%dT%H:%M:%SZ"
)
logger = logging.getLogger(__name__)


@asynccontextmanager
async def lifespan(app: FastAPI):
    logger.info("Loading embedding model...")
    state.model = SentenceTransformer("all-MiniLM-L6-v2")
    logger.info("Embedding model loaded")

    ensure_collection(get_client())
    logger.info("Qdrant collection ready")
    yield


app = FastAPI(title="ArXiv ML Service", version="0.1.0", lifespan=lifespan)

Instrumentator().instrument(app).expose(app)

app.include_router(embed.router)
app.include_router(chunk.router)
app.include_router(ingest.router)
app.include_router(papers.router)
app.include_router(query.router)


@app.middleware("http")
async def add_request_id(request: Request, call_next):
    request_id = request.headers.get("X-Request-ID") or str(uuid.uuid4())
    request.state.request_id = request_id
    response = await call_next(request)
    response.headers["X-Request-ID"] = request_id
    return response


@app.middleware("http")
async def internal_service_auth(request: Request, call_next):
    exempt = {"/health", "/metrics", "/docs", "/openapi.json", "/favicon.ico"}
    if request.url.path in exempt:
        return await call_next(request)

    expected = os.getenv("INTERNAL_SERVICE_TOKEN", "").strip()
    if not expected:
        return JSONResponse(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            content={"detail": "service not configured"},
        )
    got = request.headers.get("X-Internal-Service-Token", "").strip()
    if got != expected:
        return JSONResponse(
            status_code=status.HTTP_401_UNAUTHORIZED,
            content={"detail": "invalid internal service token"},
        )
    return await call_next(request)


@app.get("/health")
def health():
    return {
        "status": "ok",
        "model": "all-MiniLM-L6-v2",
        "model_loaded": state.model is not None,
    }
