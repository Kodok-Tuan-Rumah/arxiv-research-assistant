import logging
import re

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, Field, field_validator, model_validator

import state as app_state
from engine.db import delete_paper, get_connection, insert_chunks, upsert_paper
from engine.ingestion.arxiv_fetcher import fetch_metadata
from engine.ingestion.chunker import chunk_text
from engine.ingestion.pdf_parser import download_and_parse_pdf
from engine.retrieval.qdrant_client import (
    compute_point_ids,
    delete_by_arxiv_id,
    ensure_collection,
    get_client,
    upsert_chunks,
)

logger = logging.getLogger(__name__)
router = APIRouter()

ARXIV_ID_PATTERN = re.compile(r"^\d{4}\.\d{4,5}(v\d+)?$")

class IngestRequest(BaseModel):
    arxiv_ids: list[str] = Field(..., max_length=50)
    chunk_size: int = Field(512, gt=0, le=10_000)
    overlap: int = Field(50, ge=0)

    @model_validator(mode="after")
    def _validate_overlap(self):
        if self.overlap >= self.chunk_size:
            raise ValueError("overlap must be < chunk_size")
        return self

    @field_validator("arxiv_ids")
    @classmethod
    def _validate_arxiv_ids(cls, value: list[str]) -> list[str]:
        invalid = [arxiv_id for arxiv_id in value if not ARXIV_ID_PATTERN.match(arxiv_id)]
        if invalid:
            raise ValueError(f"invalid arxiv id(s): {', '.join(invalid[:5])}")
        return value

class IngestResponse(BaseModel):
    ingested: list[str]
    failed: list[str]
    total_chunks: int

@router.post("/ingest", response_model=IngestResponse)
def ingest(request: IngestRequest):
    if not request.arxiv_ids:
        raise HTTPException(status_code=400, detail="arxiv_ids cannot be empty")
    if app_state.model is None:
        raise HTTPException(status_code=503, detail="model not loaded")

    qdrant = get_client()
    ensure_collection(qdrant)

    ingested = []
    failed = []
    total_chunks = 0

    papers = fetch_metadata(request.arxiv_ids)
    if not papers:
        raise HTTPException(status_code=404, detail="No papers found")

    conn = get_connection()
    try:
        for paper in papers:
            paper_id = None
            pg_committed = False
            try:
                # Parse PDF
                text = download_and_parse_pdf(paper.pdf_url)

                # Chunk
                chunks = chunk_text(text, request.chunk_size, request.overlap)

                # Embed
                embeddings = app_state.model.encode(chunks).tolist()

                # Deterministic ids, shared by PG rows and Qdrant points (T4)
                qdrant_ids = compute_point_ids(paper.arxiv_id, len(chunks))

                chunk_dicts = [
                    {
                        "text": chunk,
                        "embedding": embeddings[i],
                        "arxiv_id": paper.arxiv_id,
                        "categories": paper.categories,
                    }
                    for i, chunk in enumerate(chunks)
                ]

                # Order matters: commit PostgreSQL first (source of truth), then
                # write Qdrant vectors. If the vector write fails afterwards, the
                # PG rows are removed below (compensation); the reverse order
                # would leave vectors without a paper behind on any later error.
                paper_id = upsert_paper(conn, paper)
                insert_chunks(conn, paper_id, chunks, qdrant_ids)
                conn.commit()
                pg_committed = True

                # Purge stale points, then upsert with the same precomputed ids
                delete_by_arxiv_id(qdrant, paper.arxiv_id)
                upsert_chunks(qdrant, chunk_dicts, ids=qdrant_ids)

                ingested.append(paper.arxiv_id)
                total_chunks += len(chunks)
                logger.info(f"✅ Ingested {paper.arxiv_id}: {len(chunks)} chunks")

            except Exception as e:  # noqa: BLE001 — keep ingesting the remaining papers
                if pg_committed:
                    # Qdrant failed after the PG commit → remove the PG rows
                    try:
                        delete_paper(conn, paper_id)
                        conn.commit()
                        logger.error(f"❌ {paper.arxiv_id}: vector write failed, PG rows compensated: {e}")
                    except Exception as ce:  # noqa: BLE001 — compensation is best effort
                        conn.rollback()
                        logger.error(f"❌ {paper.arxiv_id}: compensation failed: {ce}")
                else:
                    conn.rollback()
                    logger.error(f"❌ Failed to ingest {paper.arxiv_id}: {e}")
                failed.append(paper.arxiv_id)
    finally:
        conn.close()

    return IngestResponse(
        ingested=ingested,
        failed=failed,
        total_chunks=total_chunks
    )
