import logging
import os
import time
from typing import Any

from groq import Groq

logger = logging.getLogger(__name__)

DEFAULT_MODEL = "llama-3.3-70b-versatile"

class GroqError(Exception):
    pass

def build_prompt(query: str, context_chunks: list[str]) -> str:
    safe_chunks = [
        chunk.replace("\n", " ").strip()[:500]
        for chunk in context_chunks[:5]
        if chunk.strip()
    ]
    context = "\n---\n".join(safe_chunks)
    return (
        "You are a research assistant. Answer the question based on the context below.\n"
        "Be concise and accurate. If the context does not contain enough information, "
        "say what you can infer and note the limitation.\n\n"
        f"Context:\n{context}\n\n"
        f"Question: {query}\n\n"
        "Answer:"
    )

def generate(
    query: str,
    context_chunks: list[str],
    model: str = DEFAULT_MODEL,
) -> dict[str, Any]:
    api_key = os.getenv("GROQ_API_KEY")
    if not api_key:
        raise GroqError("GROQ_API_KEY not set")

    if not query.strip():
        raise ValueError("Query is empty")

    prompt = build_prompt(query, context_chunks)
    start = time.time()

    try:
        client = Groq(api_key=api_key)
        response = client.chat.completions.create(
            model=model,
            messages=[{"role": "user", "content": prompt}],
            max_tokens=300,
            temperature=0.1,
        )
        answer = response.choices[0].message.content.strip()
    except Exception as e:
        logger.exception("Groq generation failed")
        raise GroqError(f"groq_error: {e}") from e

    latency_ms = int((time.time() - start) * 1000)

    if not answer:
        answer = "Not found in context."

    logger.info(
        "Groq generation | model=%s latency=%dms chars=%d",
        model, latency_ms, len(answer),
    )

    return {
        "answer": answer,
        "latency_ms": latency_ms,
        "model": model,
        "source": "groq",
    }
