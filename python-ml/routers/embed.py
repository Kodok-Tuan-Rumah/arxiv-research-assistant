from fastapi import APIRouter, HTTPException
from pydantic import BaseModel

import state as app_state

router = APIRouter()

class EmbedRequest(BaseModel):
    texts: list[str]

class EmbedResponse(BaseModel):
    embeddings: list[list[float]]
    model: str
    count: int

@router.post("/embed", response_model=EmbedResponse)
def embed(request: EmbedRequest):
    if not request.texts:
        raise HTTPException(status_code=400, detail="texts cannot be empty")

    if app_state.model is None:
        raise HTTPException(status_code=503, detail="model not loaded")

    embeddings = app_state.model.encode(request.texts).tolist()

    return EmbedResponse(
        embeddings=embeddings,
        model="all-MiniLM-L6-v2",
        count=len(embeddings)
    )
