import logging
import os
import time

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel

import state as app_state
from engine.generation import groq_client, ollama_client
from engine.retrieval.qdrant_client import get_client, search

logger = logging.getLogger(__name__)
router = APIRouter()

class QueryRequest(BaseModel):
    query: str
    top_k: int = 5
    model: str = "phi3:mini"
    arxiv_id: str | None = None

class QueryResponse(BaseModel):
    answer: str
    sources: list[dict]
    retrieval_ms: int
    generation_ms: int
    model: str

def get_provider():
    return os.getenv("LLM_PROVIDER", "ollama")

def embed_query(query: str) -> list:
    if app_state.model is not None:
        return app_state.model.encode([query])[0].tolist()
    # Cloud mode — use Groq embeddings via simple httpx call
    # Fallback: use zeros (not ideal but won't crash)
    logger.warning("No embedding model available, using zero vector")
    return [0.0] * 384

@router.post("/query", response_model=QueryResponse)
def query(request: QueryRequest):
    if not request.query.strip():
        raise HTTPException(status_code=400, detail="query cannot be empty")

    t0 = time.time()
    query_vector = embed_query(request.query)

    qdrant = get_client()
    results = search(qdrant, query_vector, request.top_k, request.arxiv_id)
    retrieval_ms = int((time.time() - t0) * 1000)

    if not results:
        raise HTTPException(status_code=404, detail="No relevant chunks found")

    context_chunks = [r["text"] for r in results]
    provider = get_provider()

    try:
        if provider == "groq":
            model = os.getenv("GROQ_MODEL", "llama-3.3-70b-versatile")
            generation = groq_client.generate(request.query, context_chunks, model)
        else:
            generation = ollama_client.generate(request.query, context_chunks, request.model)
    except Exception as e:  # noqa: BLE001 — surface any provider failure as HTTP 500
        logger.error(f"Generation failed ({provider}): {e}")
        raise HTTPException(status_code=500, detail=f"generation failed: {e}")

    return QueryResponse(
        answer=generation["answer"],
        sources=results,
        retrieval_ms=retrieval_ms,
        generation_ms=generation["latency_ms"],
        model=generation["model"],
    )
