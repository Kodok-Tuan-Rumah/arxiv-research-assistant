import logging

import state as app_state
from engine.generation.ollama_client import generate

logger = logging.getLogger(__name__)

def evaluate_faithfulness(query: str, answer: str, context_chunks: list[str]) -> dict:
    """
    Faithfulness: apakah setiap klaim dalam answer bisa ditemukan di context?
    Score 0.0 - 1.0
    """
    context = "\n\n".join([f"[{i+1}] {chunk[:300]}" for i, chunk in enumerate(context_chunks[:3])])

    prompt = f"""You are an evaluation system. Your job is to check if an answer is faithful to the given context.

Context:
{context}

Question: {query}
Answer: {answer}

Instructions:
1. List each factual claim in the answer
2. For each claim, check if it's supported by the context
3. Calculate faithfulness score = supported_claims / total_claims

Respond in this exact format:
CLAIMS:
- [claim 1]: SUPPORTED or NOT_SUPPORTED
- [claim 2]: SUPPORTED or NOT_SUPPORTED

SCORE: [number between 0.0 and 1.0]
FAILURE_MODE: [one of: correct, hallucination, insufficient_context, mixed]"""

    result = generate(query, [prompt], model="phi3:mini")
    raw = result["answer"]

    # Parse score
    score = 0.5
    failure_mode = "unknown"

    for line in raw.split("\n"):
        if line.startswith("SCORE:"):
            try:
                score = float(line.replace("SCORE:", "").strip())
            except ValueError:
                pass
        if line.startswith("FAILURE_MODE:"):
            failure_mode = line.replace("FAILURE_MODE:", "").strip().lower()

    logger.info(f"Faithfulness score: {score}, failure_mode: {failure_mode}")

    return {
        "faithfulness": score,
        "failure_mode": failure_mode,
        "raw_evaluation": raw,
    }