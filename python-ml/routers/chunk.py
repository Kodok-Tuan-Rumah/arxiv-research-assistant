from fastapi import APIRouter, HTTPException
from pydantic import BaseModel

router = APIRouter()

class ChunkRequest(BaseModel):
    text: str
    chunk_size: int = 512
    overlap: int = 50

class ChunkResponse(BaseModel):
    chunks: list[str]
    stats: dict

@router.post("/chunk", response_model=ChunkResponse)
def chunk(request: ChunkRequest):
    if not request.text.strip():
        raise HTTPException(status_code=400, detail="text cannot be empty")

    text = request.text
    size = request.chunk_size
    overlap = request.overlap
    chunks = []
    start = 0

    while start < len(text):
        end = start + size
        chunk = text[start:end]
        if chunk.strip():
            chunks.append(chunk)
        start += size - overlap

    return ChunkResponse(
        chunks=chunks,
        stats={
            "total_chunks": len(chunks),
            "chunk_size": size,
            "overlap": overlap,
            "total_chars": len(text)
        }
    )
