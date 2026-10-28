import logging

import state as app_state
from engine.arxiv_id import normalize_arxiv_id
from engine.db import get_connection, insert_chunks, upsert_paper
from engine.ingestion.arxiv_fetcher import fetch_metadata
from engine.ingestion.chunker import chunk_text
from engine.ingestion.pdf_parser import download_and_parse_pdf
from engine.retrieval.qdrant_client import (
    compute_point_ids,
    delete_stale_chunks,
    ensure_collection,
    get_client,
    restore_snapshot,
    snapshot_by_arxiv_id,
    upsert_chunks,
)
from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, Field, field_validator, model_validator

logger = logging.getLogger(__name__)
router = APIRouter()


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
        normalized = [normalize_arxiv_id(v) for v in value]
        return list(dict.fromkeys(normalized))


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
            snapshot = None
            try:
                text = download_and_parse_pdf(paper.pdf_url)
                chunks = chunk_text(text, request.chunk_size, request.overlap)
                if not chunks:
                    raise ValueError("no_chunks_extracted")
                embeddings = app_state.model.encode(chunks).tolist()
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

                snapshot = snapshot_by_arxiv_id(qdrant, paper.arxiv_id)
                upsert_chunks(qdrant, chunk_dicts, ids=qdrant_ids)

                paper_id = upsert_paper(conn, paper)
                insert_chunks(conn, paper_id, chunks, qdrant_ids)
                conn.commit()

                try:
                    delete_stale_chunks(qdrant, paper.arxiv_id, qdrant_ids)
                except Exception as stale_exc:  # noqa: BLE001 — sweep is best effort
                    logger.warning(
                        f"[WARING] {paper.arxiv_id}: stale vector sweep failed "
                        f"(unchanged, reconciliation will handle it): {stale_exc}"
                    )

                ingested.append(paper.arxiv_id)
                total_chunks += len(chunks)
                logger.info(f"[OK] Ingested {paper.arxiv_id}: {len(chunks)} chunks")

            except Exception as e:  # noqa: BLE001 — keep ingesting the remaining papers
                try:
                    conn.rollback()
                    if snapshot is not None:
                        restore_snapshot(qdrant, paper.arxiv_id, snapshot)
                    logger.error(f"[ERROR] Failed to ingest {paper.arxiv_id}: {e}")
                except Exception as ce:  # noqa: BLE001 — compensation is best effort
                    logger.error(
                        f"[ERROR] {paper.arxiv_id}: rollback/restore path failed: "
                        f"orig={e!r}, compensation={ce}"
                    )
                failed.append(paper.arxiv_id)
    finally:
        conn.close()

    return IngestResponse(ingested=ingested, failed=failed, total_chunks=total_chunks)
