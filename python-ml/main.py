import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI
from prometheus_fastapi_instrumentator import Instrumentator
from sentence_transformers import SentenceTransformer

import state
from routers import chunk, embed, evaluate, ingest, papers, query

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

@asynccontextmanager
async def lifespan(app: FastAPI):
    logger.info("Loading embedding model...")
    state.model = SentenceTransformer("all-MiniLM-L6-v2")
    logger.info("Embedding model loaded")
    yield

app = FastAPI(title="ArXiv ML Service", version="0.1.0", lifespan=lifespan)

Instrumentator().instrument(app).expose(app)

app.include_router(embed.router)
app.include_router(chunk.router)
app.include_router(ingest.router)
app.include_router(papers.router)
app.include_router(query.router)
app.include_router(evaluate.router)

@app.get("/health")
def health():
    return {
        "status": "ok",
        "model": "all-MiniLM-L6-v2",
        "model_loaded": state.model is not None
    }
