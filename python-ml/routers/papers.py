import logging

from engine.retrieval.qdrant_client import (
    COLLECTION_NAME,
    delete_by_arxiv_id,
    get_client,
)
from fastapi import APIRouter

logger = logging.getLogger(__name__)
router = APIRouter()


@router.delete("/papers/{arxiv_id}")
def delete_paper_vectors(arxiv_id: str):
    qdrant = get_client()
    collections = [c.name for c in qdrant.get_collections().collections]
    if COLLECTION_NAME in collections:
        delete_by_arxiv_id(qdrant, arxiv_id)
        logger.info(f"Deleted vectors for {arxiv_id}")
    else:
        logger.info(
            f"Collection {COLLECTION_NAME} absent; nothing to delete for {arxiv_id}"
        )
    return {"deleted": True, "arxiv_id": arxiv_id}
