import logging

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel

from engine.evaluation.faithfulness import evaluate_faithfulness

logger = logging.getLogger(__name__)
router = APIRouter()

class EvaluateRequest(BaseModel):
    query: str
    answer: str
    context_chunks: list[str]

class EvaluateResponse(BaseModel):
    faithfulness: float
    failure_mode: str
    raw_evaluation: str

@router.post("/evaluate", response_model=EvaluateResponse)
def evaluate(request: EvaluateRequest):
    if not request.query or not request.answer:
        raise HTTPException(status_code=400, detail="query and answer are required")

    result = evaluate_faithfulness(
        request.query,
        request.answer,
        request.context_chunks
    )

    return EvaluateResponse(
        faithfulness=result["faithfulness"],
        failure_mode=result["failure_mode"],
        raw_evaluation=result["raw_evaluation"],
    )
