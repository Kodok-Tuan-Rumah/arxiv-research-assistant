import logging
import uuid

from qdrant_client import QdrantClient
from qdrant_client.models import Distance, PointStruct, VectorParams

logger = logging.getLogger(__name__)

COLLECTION_NAME = "arxiv_chunks"
VECTOR_SIZE = 384

def get_client(host: str = "localhost", port: int = 6333) -> QdrantClient:
    return QdrantClient(host=host, port=port)

def ensure_collection(client: QdrantClient):
    collections = [c.name for c in client.get_collections().collections]
    if COLLECTION_NAME not in collections:
        client.create_collection(
            collection_name=COLLECTION_NAME,
            vectors_config=VectorParams(size=VECTOR_SIZE, distance=Distance.COSINE),
        )
        logger.info(f"Created collection: {COLLECTION_NAME}")
    else:
        logger.info(f"Collection already exists: {COLLECTION_NAME}")

def compute_point_ids(arxiv_id: str, count: int) -> list[str]:
    """Deterministic point ids: same paper + position always yields the same id."""
    return [
        str(uuid.uuid5(uuid.NAMESPACE_URL, f"{arxiv_id}:{i}"))
        for i in range(count)
    ]

def upsert_chunks(client: QdrantClient, chunks: list[dict], ids: list[str] | None = None) -> list[str]:
    if not chunks:
        return []

    if ids is None:
        ids = compute_point_ids(chunks[0].get("arxiv_id", ""), len(chunks))
    elif len(ids) != len(chunks):
        raise ValueError("ids must have the same length as chunks")

    points = [
        PointStruct(
            id=ids[i],
            vector=chunk["embedding"],
            payload={
                "chunk_id": chunk.get("chunk_id", ""),
                "paper_id": chunk.get("paper_id", ""),
                "arxiv_id": chunk.get("arxiv_id", ""),
                "text": chunk["text"],
                "categories": chunk.get("categories", []),
            }
        )
        for i, chunk in enumerate(chunks)
    ]

    client.upsert(collection_name=COLLECTION_NAME, points=points)
    logger.info(f"Upserted {len(points)} chunks to Qdrant")
    return ids

def delete_by_arxiv_id(client: QdrantClient, arxiv_id: str) -> None:
    """Remove all stored vectors of a paper (called before re-ingest to avoid duplicates)."""
    from qdrant_client.models import FieldCondition, Filter, FilterSelector, MatchValue

    client.delete(
        collection_name=COLLECTION_NAME,
        points_selector=FilterSelector(
            filter=Filter(
                must=[FieldCondition(key="arxiv_id", match=MatchValue(value=arxiv_id))]
            )
        ),
    )
    logger.info(f"Deleted stale vectors for {arxiv_id}")

def search(client: QdrantClient, query_vector: list[float], top_k: int = 5, arxiv_id: str | None = None):
    from qdrant_client.models import FieldCondition, Filter, MatchValue

    query_filter = None
    if arxiv_id:
        query_filter = Filter(
            must=[FieldCondition(key="arxiv_id", match=MatchValue(value=arxiv_id))]
        )

    results = client.query_points(
        collection_name=COLLECTION_NAME,
        query=query_vector,
        limit=top_k,
        query_filter=query_filter,
    ).points

    return [
        {
            "text": r.payload["text"],
            "score": r.score,
            "arxiv_id": r.payload.get("arxiv_id"),
        }
        for r in results
    ]
