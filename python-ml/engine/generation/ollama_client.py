import logging
import os
import time
from typing import Any

import httpx

logger = logging.getLogger(__name__)

OLLAMA_URL = os.getenv("OLLAMA_HOST", "http://localhost:11434")
DEFAULT_MODEL = "phi3:mini"

class OllamaError(Exception):
    pass

def build_prompt(query: str, context_chunks: list[str]) -> str:
    safe_chunks = [
        chunk.replace("\n", " ").strip()[:800]
        for chunk in context_chunks[:5]
        if chunk.strip()
    ]
    context = "\n---\n".join(safe_chunks)
    return (
        "You are a helpful research assistant. Answer the question using the context below.\n"
        "Be concise. If the context contains relevant information, use it to answer.\n"
        "Only say 'Not found in context.' if the context has absolutely no relevant information.\n\n"
        f"Context:\n{context}\n\n"
        f"Question: {query}\n\n"
        "Answer:"
    )

def generate(
    query: str,
    context_chunks: list[str],
    model: str = DEFAULT_MODEL,
) -> dict[str, Any]:

    if not query.strip():
        raise ValueError("Query is empty")

    prompt = build_prompt(query, context_chunks)
    start = time.time()

    try:
        with httpx.Client(
            timeout=httpx.Timeout(600.0, connect=10.0)
        ) as client:
            response = client.post(
                f"{OLLAMA_URL}/api/generate",
                json={
                    "model": model,
                    "prompt": prompt,
                    "stream": False,
                    "options": {
                        "num_predict": 150,
                        "temperature": 0.1,
                    },
                },
            )
            response.raise_for_status()
            payload = response.json()

    except httpx.ConnectError as e:
        logger.error("Ollama service unreachable")
        raise OllamaError("ollama_unreachable") from e

    except httpx.HTTPStatusError as e:
        logger.error(f"Ollama HTTP error: {e.response.text}")
        raise OllamaError("ollama_http_error") from e

    except Exception as e:
        logger.exception("Unexpected Ollama failure")
        raise OllamaError("ollama_unknown_error") from e

    latency_ms = int((time.time() - start) * 1000)

    answer = payload.get("response", "").strip()
    if not answer:
        answer = "Not found in context."

    logger.info(
        "Ollama generation | model=%s latency=%dms chars=%d",
        model,
        latency_ms,
        len(answer),
    )

    return {
        "answer": answer,
        "latency_ms": latency_ms,
        "model": model,
        "source": "ollama",
    }