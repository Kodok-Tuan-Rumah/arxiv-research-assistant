from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, Field, model_validator

from engine.ingestion.chunker import MAX_TEXT_CHARS, chunk_text

router = APIRouter()

class ChunkRequest(BaseModel):
    text: str = Field(..., max_length=MAX_TEXT_CHARS)
    chunk_size: int = Field(512, gt=0, le=10_000)
    overlap: int = Field(50, ge=0)

    @model_validator(mode="after")
    def _validate_overlap(self):
        if self.overlap >= self.chunk_size:
            raise ValueError("overlap must be < chunk_size")
        return self

class ChunkResponse(BaseModel):
    chunks: list[str]
    stats: dict

@router.post("/chunk", response_model=ChunkResponse)
def chunk(request: ChunkRequest):
    if not request.text.strip():
        raise HTTPException(status_code=400, detail="text cannot be empty")

    chunks = chunk_text(request.text, request.chunk_size, request.overlap)

    return ChunkResponse(
        chunks=chunks,
        stats={
            "total_chunks": len(chunks),
            "chunk_size": request.chunk_size,
            "overlap": request.overlap,
            "total_chars": len(request.text),
        }
    )
