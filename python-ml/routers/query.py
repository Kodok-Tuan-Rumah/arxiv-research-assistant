import logging
import os
import time

from fastapi import APIRouter, HTTPException, Request
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
    if app_state.model is None:
        # Startup loads the model unconditionally; this guard is defensive.
        raise HTTPException(status_code=503, detail="embedding model not loaded")
    return app_state.model.encode([query])[0].tolist()

@router.post("/query", response_model=QueryResponse)
def query(payload: QueryRequest, request: Request):
    request_id = request.headers.get("x-request-id", "-")

    if not payload.query.strip():
        raise HTTPException(status_code=400, detail="query cannot be empty")

    logger.info(
        "[%s] query received (chars=%d, top_k=%d)",
        request_id,
        len(payload.query),
        payload.top_k,
    )

    t0 = time.time()
    query_vector = embed_query(payload.query)

    qdrant = get_client()
    results = search(qdrant, query_vector, payload.top_k, payload.arxiv_id)
    retrieval_ms = int((time.time() - t0) * 1000)

    if not results:
        raise HTTPException(status_code=404, detail="No relevant chunks found")

    context_chunks = [r["text"] for r in results]
    provider = get_provider()

    try:
        if provider == "groq":
            model = os.getenv("GROQ_MODEL", "llama-3.3-70b-versatile")
            generation = groq_client.generate(payload.query, context_chunks, model)
        else:
            generation = ollama_client.generate(payload.query, context_chunks, payload.model)
    except Exception:
        logger.exception("[%s] Generation failed (%s)", request_id, provider)
        raise HTTPException(status_code=500, detail="generation failed")

    return QueryResponse(
        answer=generation["answer"],
        sources=results,
        retrieval_ms=retrieval_ms,
        generation_ms=generation["latency_ms"],
        model=generation["model"],
    )
