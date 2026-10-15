import logging

logger = logging.getLogger(__name__)

MAX_TEXT_CHARS = 1_000_000

def chunk_text(text: str, chunk_size: int = 512, overlap: int = 50) -> list[str]:
    if chunk_size <= 0:
        raise ValueError("chunk_size must be > 0")
    if overlap < 0 or overlap >= chunk_size:
        raise ValueError("overlap must satisfy 0 <= overlap < chunk_size")
    if len(text) > MAX_TEXT_CHARS:
        raise ValueError(f"text too large (max {MAX_TEXT_CHARS} chars)")

    chunks = []
    start = 0
    
    while start < len(text):
        end = start + chunk_size
        chunk = text[start:end].strip()
        if chunk:
            chunks.append(chunk)
        start += chunk_size - overlap
    
    logger.info(f"Chunked text into {len(chunks)} chunks")
    return chunks
